package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"chunsu/internal/files"
)

const (
	ResultAvailable        = "available"
	ResultSending          = "sending"
	ResultDelivered        = "delivered"
	ResultUnconfirmed      = "unconfirmed"
	ResultDeliveryFailed   = "failed"
	MaxResultSummaryBytes  = 4096
	MaxResultMetadataBytes = 16384
)

type ResultEventInput struct {
	JobID            string          `json:"job_id"`
	StepID           string          `json:"step_id"`
	ArtifactID       string          `json:"artifact_id"`
	Summary          string          `json:"summary"`
	SourceReferences json.RawMessage `json:"source_references"`
	Gaps             json.RawMessage `json:"gaps"`
	Status           string          `json:"status"`
	RequiredDecision json.RawMessage `json:"required_decision"`
}

type ResultEvent struct {
	ID               string          `json:"id"`
	EventKey         string          `json:"event_key"`
	JobID            string          `json:"job_id"`
	StepID           string          `json:"step_id"`
	ConversationID   string          `json:"conversation_id"`
	RequestRevision  int64           `json:"request_revision"`
	ArtifactID       string          `json:"artifact_id"`
	ArtifactDigest   string          `json:"artifact_digest"`
	Summary          string          `json:"summary"`
	SourceReferences json.RawMessage `json:"source_references"`
	Gaps             json.RawMessage `json:"gaps"`
	Status           string          `json:"status"`
	RequiredDecision json.RawMessage `json:"required_decision"`
	DeliveryStatus   string          `json:"delivery_status"`
	CreatedAt        int64           `json:"created_at"`
	UpdatedAt        int64           `json:"updated_at"`
}

const resultEventColumns = "id,event_key,job_id,step_id,conversation_id,request_revision,artifact_id,artifact_digest,summary,source_refs_json,gaps_json,status,required_decision_json,delivery_status,created_at,updated_at"

func scanResultEvent(row scanner) (ResultEvent, error) {
	var e ResultEvent
	var sourceRefs, gaps, decision string
	err := row.Scan(&e.ID, &e.EventKey, &e.JobID, &e.StepID, &e.ConversationID, &e.RequestRevision, &e.ArtifactID, &e.ArtifactDigest, &e.Summary, &sourceRefs, &gaps, &e.Status, &decision, &e.DeliveryStatus, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return e, err
	}
	e.SourceReferences = json.RawMessage(sourceRefs)
	e.Gaps = json.RawMessage(gaps)
	e.RequiredDecision = json.RawMessage(decision)
	return e, nil
}

func validEventJSON(value json.RawMessage) bool {
	return len(value) == 0 || json.Valid(value) && len(value) <= MaxResultMetadataBytes
}

// PublishResultEvent is idempotent for one validated step output and request
// revision. It checks artifact identity and file integrity before handoff.
func (s *Store) PublishResultEvent(ctx context.Context, input ResultEventInput) (ResultEvent, error) {
	if input.JobID == "" || input.StepID == "" || input.ArtifactID == "" || input.Status == "" || len(input.Summary) > MaxResultSummaryBytes || !validEventJSON(input.SourceReferences) || !validEventJSON(input.Gaps) || !validEventJSON(input.RequiredDecision) {
		return ResultEvent{}, errors.New("invalid or oversized result event")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ResultEvent{}, err
	}
	defer tx.Rollback()
	var conversationID string
	var revision int64
	var jobStatus string
	err = tx.QueryRowContext(ctx, "SELECT w.origin_conversation_id,w.request_revision,j.status FROM workflow_jobs w JOIN jobs j ON j.id=w.job_id WHERE w.job_id=?", input.JobID).Scan(&conversationID, &revision, &jobStatus)
	if err != nil {
		return ResultEvent{}, err
	}
	if jobStatus != Staged && jobStatus != Completed {
		return ResultEvent{}, ErrStaleStep
	}
	step, err := scanStep(tx.QueryRowContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE id=? AND job_id=?", input.StepID, input.JobID))
	if err != nil {
		return ResultEvent{}, err
	}
	if step.State != StepCompleted || step.OutputArtifactID != input.ArtifactID {
		return ResultEvent{}, ErrStaleStep
	}
	var artifact Artifact
	err = tx.QueryRowContext(ctx, "SELECT id,job_id,attempt_id,kind,path,digest,bytes,created_at,content_state FROM artifacts WHERE id=? AND job_id=?", input.ArtifactID, input.JobID).Scan(&artifact.ID, &artifact.JobID, &artifact.AttemptID, &artifact.Kind, &artifact.Path, &artifact.Digest, &artifact.Bytes, &artifact.CreatedAt, &artifact.ContentState)
	if err != nil {
		return ResultEvent{}, err
	}
	if _, err = s.ReadArtifact(artifact, artifact.Bytes); err != nil {
		return ResultEvent{}, err
	}
	e := ResultEvent{ID: files.ID(), EventKey: fmt.Sprintf("%s:%s:%d:%s", input.JobID, input.StepID, revision, input.ArtifactID), JobID: input.JobID, StepID: input.StepID, ConversationID: conversationID, RequestRevision: revision, ArtifactID: input.ArtifactID, ArtifactDigest: artifact.Digest, Summary: input.Summary, SourceReferences: input.SourceReferences, Gaps: input.Gaps, Status: input.Status, RequiredDecision: input.RequiredDecision, DeliveryStatus: ResultAvailable, CreatedAt: now(), UpdatedAt: now()}
	var existingID string
	err = tx.QueryRowContext(ctx, "SELECT id FROM result_events WHERE event_key=?", e.EventKey).Scan(&existingID)
	if err == nil {
		existing, scanErr := scanResultEvent(tx.QueryRowContext(ctx, "SELECT "+resultEventColumns+" FROM result_events WHERE id=?", existingID))
		return existing, scanErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ResultEvent{}, err
	}
	sourceRefs := jsonValue(e.SourceReferences)
	if len(e.SourceReferences) == 0 {
		sourceRefs = "[]"
	}
	gaps := jsonValue(e.Gaps)
	if len(e.Gaps) == 0 {
		gaps = "[]"
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO result_events(id,event_key,job_id,step_id,conversation_id,request_revision,artifact_id,artifact_digest,summary,source_refs_json,gaps_json,status,required_decision_json,delivery_status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", e.ID, e.EventKey, e.JobID, e.StepID, e.ConversationID, e.RequestRevision, e.ArtifactID, e.ArtifactDigest, e.Summary, sourceRefs, gaps, e.Status, jsonValue(e.RequiredDecision), e.DeliveryStatus, e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return ResultEvent{}, err
	}
	data, _ := json.Marshal(map[string]string{"event_id": e.ID, "artifact_id": e.ArtifactID})
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,kind,created_at,data_json) VALUES(?,?,?,?)", e.JobID, "result.available", now(), string(data)); err != nil {
		return ResultEvent{}, err
	}
	return e, tx.Commit()
}

func (s *Store) ResultEvent(ctx context.Context, eventID string) (ResultEvent, error) {
	return scanResultEvent(s.DB.QueryRowContext(ctx, "SELECT "+resultEventColumns+" FROM result_events WHERE id=?", eventID))
}

// PendingResultEvents excludes sending and unconfirmed events so uncertain
// transport outcomes are not replayed after a restart.
func (s *Store) PendingResultEvents(ctx context.Context, conversationID string) ([]ResultEvent, error) {
	return s.resultEvents(ctx, conversationID, ResultAvailable)
}

func (s *Store) ResultEvents(ctx context.Context, conversationID string) ([]ResultEvent, error) {
	return s.resultEvents(ctx, conversationID, "")
}

func (s *Store) resultEvents(ctx context.Context, conversationID, deliveryStatus string) ([]ResultEvent, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+resultEventColumns+" FROM result_events WHERE conversation_id=? AND (?='' OR delivery_status=?) ORDER BY created_at,id", conversationID, deliveryStatus, deliveryStatus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResultEvent{}
	for rows.Next() {
		e, scanErr := scanResultEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func allowedDeliveryTransition(from, to string) bool {
	switch from {
	case ResultAvailable:
		return to == ResultSending || to == ResultDelivered
	case ResultSending:
		return to == ResultDelivered || to == ResultUnconfirmed || to == ResultDeliveryFailed
	case ResultUnconfirmed:
		return to == ResultDelivered
	case ResultDeliveryFailed:
		return to == ResultAvailable
	}
	return false
}

func (s *Store) MarkResultDelivery(ctx context.Context, eventID, status string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old, jobID string
	if err = tx.QueryRowContext(ctx, "SELECT delivery_status,job_id FROM result_events WHERE id=?", eventID).Scan(&old, &jobID); err != nil {
		return err
	}
	if old == status {
		return nil
	}
	if !allowedDeliveryTransition(old, status) {
		return errors.New("invalid or stale result delivery transition")
	}
	result, err := tx.ExecContext(ctx, "UPDATE result_events SET delivery_status=?,updated_at=? WHERE id=? AND delivery_status=?", status, now(), eventID, old)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrStaleStep
	}
	if status == ResultDelivered {
		// Completion requires all required steps and all result handoffs.
		_, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=? WHERE id=? AND status=? AND NOT EXISTS(SELECT 1 FROM workflow_steps WHERE job_id=? AND state!=?) AND NOT EXISTS(SELECT 1 FROM result_events WHERE job_id=? AND delivery_status!=?)", Completed, now(), jobID, Staged, jobID, StepCompleted, jobID, ResultDelivered)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
