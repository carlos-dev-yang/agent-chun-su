package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/store"
)

type Service struct {
	Store  *store.Store
	Config config.Config
}

// Load reads every committed record for this scope. created_at and random IDs
// are not ordering authorities: a contiguous revision chain is mandatory.
// Callers of Execute hold the existing controller admission/writer lock.
func (s Service) Load(ctx context.Context, scope Scope) (State, []HistoryEntry, error) {
	state := emptyState(scope)
	if err := scope.Validate(); err != nil {
		return state, nil, err
	}
	rows, err := s.Store.DB.QueryContext(ctx, "SELECT id,kind,subject_id,path,digest,bytes,created_at FROM records WHERE kind=? AND subject_id=?", RecordKind, scope.Key())
	if err != nil {
		return state, nil, err
	}
	records := []store.Record{}
	var totalBytes int64
	for rows.Next() {
		var record store.Record
		if err = rows.Scan(&record.ID, &record.Kind, &record.SubjectID, &record.Path, &record.Digest, &record.Bytes, &record.CreatedAt); err != nil {
			rows.Close()
			return state, nil, err
		}
		if record.Bytes <= 0 || record.Bytes > s.Config.Limits.MaxEvidenceBytes-totalBytes {
			rows.Close()
			return state, nil, errors.New("OPS complete ledger replay exceeds max_evidence_bytes; history is retained and no partial state was used")
		}
		totalBytes += record.Bytes
		records = append(records, record)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil || closeErr != nil {
		return state, nil, errors.Join(err, closeErr)
	}
	history := make([]HistoryEntry, 0, len(records))
	for _, record := range records {
		data, readErr := s.Store.ReadRecord(record, s.Config.Limits.MaxArtifactBytes)
		if readErr != nil {
			return state, nil, fmt.Errorf("OPS ledger record %s: %w", record.ID, readErr)
		}
		var event Event
		if err = decode(data, &event); err != nil {
			return state, nil, err
		}
		history = append(history, HistoryEntry{Record: record, Event: event})
	}
	sort.Slice(history, func(i, j int) bool { return history[i].Event.Revision < history[j].Event.Revision })
	operations := map[string]bool{}
	for _, entry := range history {
		event := entry.Event
		command := event.Command
		commandData, _ := json.Marshal(command)
		if event.Version != Version || event.Scope != scope || command.Scope != scope || event.Revision != state.Revision+1 || event.Previous != state.Head || event.CommandDigest != files.Digest(commandData) || command.ExpectedRevision != state.Revision || operations[command.OperationID] || !timestamp(event.At) {
			return state, nil, errors.New("OPS ledger integrity failed: scope, revision, parent or operation mismatch")
		}
		if err = validateCommand(command, s.Config.Limits); err != nil {
			return state, nil, err
		}
		if err = apply(&state, command, event.At, s.Config.Limits); err != nil {
			return state, nil, fmt.Errorf("OPS ledger revision %d: %w", event.Revision, err)
		}
		state.Revision = event.Revision
		state.Head = RecordRef{ID: entry.Record.ID, Digest: entry.Record.Digest}
		operations[command.OperationID] = true
	}
	return state, history, nil
}

func validateCommand(command Command, limits config.Limits) error {
	if command.Version != Version || command.Scope.Validate() != nil || !files.ValidID(command.OperationID) || command.ExpectedRevision < 0 || !nonempty(command.Actor) || !nonempty(command.Reason) || !json.Valid(command.Data) {
		return errors.New("OPS command requires version, scope, operation ID, revision, owner actor, reason and JSON data")
	}
	b, err := json.Marshal(command)
	if err != nil || int64(len(b)) > limits.MaxArtifactBytes {
		return errors.New("OPS command exceeds the configured artifact budget")
	}
	return nil
}

func (s Service) Execute(ctx context.Context, command Command) (Receipt, error) {
	if command.Action == "assessment.apply" {
		return Receipt{}, errors.New("assessment application requires ops reconcile and a verified actual FLOW result")
	}
	return s.execute(ctx, command)
}

// RecordAssessment is called by the controller after result-chain verification.
// It pins the report to the same current local scope revision before append.
func (s Service) RecordAssessment(ctx context.Context, command Command, assessment Assessment) (Receipt, error) {
	if command.Action != "assessment.apply" {
		return Receipt{}, errors.New("assessment controller action mismatch")
	}
	command.Data = mustJSON(assessment)
	return s.execute(ctx, command)
}

func (s Service) execute(ctx context.Context, command Command) (Receipt, error) {
	if err := validateCommand(command, s.Config.Limits); err != nil {
		return Receipt{}, err
	}
	state, history, err := s.Load(ctx, command.Scope)
	if err != nil {
		return Receipt{}, err
	}
	data, _ := json.Marshal(command)
	digest := files.Digest(data)
	for _, entry := range history {
		if entry.Event.Command.OperationID == command.OperationID {
			if entry.Event.CommandDigest != digest {
				return Receipt{}, errors.New("OPS operation ID was already used with a different command")
			}
			return Receipt{Record: entry.Record, Revision: entry.Event.Revision, OperationID: command.OperationID, Replayed: true}, nil
		}
	}
	if state.Revision != command.ExpectedRevision {
		return Receipt{}, fmt.Errorf("OPS revision conflict: expected %d, current %d; inspect state before applying a new command", command.ExpectedRevision, state.Revision)
	}
	if state.Revision == math.MaxInt64 {
		return Receipt{}, errors.New("OPS revision capacity exhausted")
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	if err = apply(&state, command, at, s.Config.Limits); err != nil {
		return Receipt{}, err
	}
	event := Event{Version: Version, Scope: command.Scope, Revision: state.Revision + 1, Previous: state.Head, At: at, CommandDigest: digest, Command: command}
	eventData, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return Receipt{}, err
	}
	var existingBytes int64
	for _, entry := range history {
		existingBytes += entry.Record.Bytes
	}
	if int64(len(eventData)) > s.Config.Limits.MaxEvidenceBytes-existingBytes {
		return Receipt{}, errors.New("OPS append would exceed the complete ledger replay byte budget; existing state remains readable")
	}
	record, err := s.Store.PutRecord(ctx, RecordKind, command.Scope.Key(), event, s.Config.Limits.MaxArtifactBytes)
	return Receipt{Record: record, Revision: event.Revision, OperationID: command.OperationID}, err
}
