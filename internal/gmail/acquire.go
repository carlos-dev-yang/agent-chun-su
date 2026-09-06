package gmail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
)

type AcquisitionState struct {
	Version        int            `json:"version"`
	Policy         Policy         `json:"policy"`
	PolicyDigest   string         `json:"policy_digest"`
	EffectiveQuery string         `json:"effective_query"`
	StartPageToken string         `json:"start_page_token"`
	NextPageToken  string         `json:"next_page_token"`
	Listed         bool           `json:"listed"`
	Pending        []MessageID    `json:"pending"`
	Sources        []Normalized   `json:"sources"`
	Errors         []string       `json:"errors"`
	Failures       []string       `json:"failures"`
	Snapshot       *mail.Snapshot `json:"snapshot,omitempty"`
}
type Collector struct {
	Store  *store.Store
	Config config.Config
}

func PolicyDigest(p Policy) string { b, _ := json.Marshal(p); return files.Digest(b) }
func SourceFingerprint(m mail.Message) string {
	m.Scope = ""
	b, _ := json.Marshal(m)
	return files.Digest(b)
}

func CoverageForReport(snapshot mail.Snapshot, report mail.Report) map[string]string {
	resolved := map[string]bool{}
	for _, d := range report.Dispositions {
		resolved[d.SourceID] = d.Disposition != "unresolved"
	}
	out := map[string]string{}
	for _, m := range snapshot.Messages {
		if m.Scope == mail.Target && m.ContentStatus == "complete" && len(m.Attachments) == 0 && resolved[m.ID] {
			out[m.ID] = SourceFingerprint(m)
		}
	}
	return out
}

func (c Collector) state(ctx context.Context, id string) (store.Acquisition, AcquisitionState, error) {
	var state AcquisitionState
	a, err := c.Store.Acquisition(ctx, id)
	if err != nil {
		return a, state, err
	}
	b, err := c.Store.ReadAcquisition(a, c.Config.Limits.MaxEvidenceBytes)
	if err != nil {
		return a, state, err
	}
	if err = mail.Decode(b, &state); err != nil {
		return a, state, err
	}
	if state.Version != Version || PolicyDigest(state.Policy) != state.PolicyDigest {
		return a, state, errors.New("acquisition policy integrity mismatch")
	}
	return a, state, nil
}

func (c Collector) Collect(ctx context.Context, connection Connection, resumeID, continueID string) (store.Acquisition, error) {
	var a store.Acquisition
	var state AcquisitionState
	if resumeID != "" && continueID != "" {
		return a, errors.New("choose resume or continuation")
	}
	if continueID != "" {
		existing, err := c.Store.Continuation(ctx, continueID)
		if err == nil {
			resumeID = existing.ID
			continueID = ""
		} else if !errors.Is(err, sql.ErrNoRows) {
			return a, err
		}
	}
	if resumeID != "" {
		var err error
		a, state, err = c.state(ctx, resumeID)
		if err != nil {
			return a, err
		}
		if a.ConnectionID != connection.ID || state.PolicyDigest != PolicyDigest(connection.Policy) {
			return a, errors.New("connection or policy changed; start a separate acquisition")
		}
		if state.Snapshot != nil {
			return a, nil
		}
		if a.NotBefore > time.Now().UnixMilli() {
			return a, errors.New("provider retry is not yet eligible")
		}
	} else {
		asOf := time.Now().UTC().Truncate(time.Second)
		state = AcquisitionState{Version: Version, Policy: connection.Policy, PolicyDigest: PolicyDigest(connection.Policy), Pending: []MessageID{}, Sources: []Normalized{}, Errors: []string{}}
		a = store.Acquisition{ID: files.ID(), ConnectionID: connection.ID, Status: "collecting", AsOf: asOf.Format(time.RFC3339Nano)}
		state.EffectiveQuery = "(" + connection.Policy.Query + ") before:" + strconv.FormatInt(asOf.Unix(), 10)
		if continueID != "" {
			parent, previous, err := c.state(ctx, continueID)
			if err != nil {
				return a, err
			}
			if parent.ConnectionID != connection.ID || previous.PolicyDigest != state.PolicyDigest || previous.Snapshot == nil || previous.NextPageToken == "" {
				return a, errors.New("no compatible completed page is available to continue")
			}
			a.ParentID = parent.ID
			a.AsOf = parent.AsOf
			state.StartPageToken = previous.NextPageToken
			state.EffectiveQuery = previous.EffectiveQuery
		}
		var err error
		a, err = c.Store.SaveAcquisition(ctx, a, state, c.Config.Limits.MaxEvidenceBytes)
		if err != nil {
			return a, err
		}
	}
	persist := func() error {
		var err error
		a, err = c.Store.SaveAcquisition(context.WithoutCancel(ctx), a, state, c.Config.Limits.MaxEvidenceBytes)
		return err
	}
	fail := func(err error) (store.Acquisition, error) {
		state.Failures = append(state.Failures, err.Error())
		a.Status = "failed"
		var api *APIError
		if errors.As(err, &api) {
			if api.Kind == "waiting_auth" {
				a.Status = "waiting_auth"
			}
			if strings.HasPrefix(api.Kind, "transient_") || api.Kind == "rate_limited" {
				a.Status = "retry_wait"
				a.NotBefore = time.Now().Add(time.Duration(api.RetryAfterSeconds) * time.Second).UnixMilli()
			}
		}
		persistErr := persist()
		return a, errors.Join(err, persistErr)
	}
	client, err := OpenClient(ctx, connection, c.Config)
	if err != nil {
		return fail(err)
	}
	account, err := client.Profile(ctx)
	if err != nil {
		return fail(err)
	}
	if err = CheckAccount(connection.Account, account); err != nil {
		return fail(err)
	}
	a.Status = "collecting"
	a.NotBefore = 0
	if !state.Listed {
		page, err := client.List(ctx, state.EffectiveQuery, state.StartPageToken, connection.Policy.BatchSize)
		if err != nil {
			return fail(err)
		}
		seen := map[string]bool{}
		for _, id := range page.Messages {
			if !providerID(id.ID) || !providerID(id.ThreadID) {
				return fail(errors.New("provider page contains an invalid source identity"))
			}
			if !seen[id.ID] {
				state.Pending = append(state.Pending, id)
				seen[id.ID] = true
			}
		}
		if len(state.Pending) > connection.Policy.BatchSize {
			return fail(errors.New("provider page exceeded the declared batch limit"))
		}
		state.NextPageToken = page.NextPageToken
		state.Listed = true
		if err = persist(); err != nil {
			return a, err
		}
	}
	asOf, err := time.Parse(time.RFC3339Nano, a.AsOf)
	if err != nil {
		return fail(err)
	}
	for len(state.Pending) > 0 {
		id := state.Pending[0]
		raw, e := client.Message(ctx, id.ID)
		if e != nil {
			var api *APIError
			if errors.As(e, &api) && api.Kind == "not_found" {
				state.Sources = append(state.Sources, Normalized{Message: mail.Message{ID: SourceID(connection.ID, id.ID), ThreadID: SourceID(connection.ID, id.ThreadID), Scope: mail.Target, Channel: "gmail", ContentStatus: "unavailable", Attachments: []mail.Attachment{}}, LabelIDs: []string{}, Notes: []string{e.Error()}})
				state.Errors = append(state.Errors, "message unavailable: "+id.ID)
			} else {
				return fail(e)
			}
		} else {
			normalized, e := Normalize(connection.ID, raw, mail.Target, asOf, c.Config.Limits)
			if e != nil {
				state.Errors = append(state.Errors, "message normalization unavailable: "+id.ID)
				state.Sources = append(state.Sources, Normalized{Message: mail.Message{ID: SourceID(connection.ID, id.ID), ThreadID: SourceID(connection.ID, id.ThreadID), Scope: mail.Target, Channel: "gmail", ContentStatus: "unavailable", Attachments: []mail.Attachment{}}, LabelIDs: raw.LabelIDs, Notes: []string{e.Error()}})
			} else {
				covered, e := c.Store.Covered(ctx, connection.ID, normalized.Message.ID, SourceFingerprint(normalized.Message))
				if e != nil {
					return a, e
				}
				if covered {
					normalized.Message.Scope = mail.Reference
				}
				state.Sources = append(state.Sources, normalized)
			}
		}
		state.Pending = state.Pending[1:]
		if err = persist(); err != nil {
			return a, err
		}
	}
	if connection.Policy.ThreadHistory {
		seen := map[string]bool{}
		threads := map[string]bool{}
		for _, source := range state.Sources {
			seen[source.Message.ID] = true
			threads[strings.TrimPrefix(source.Message.ThreadID, connection.ID+":")] = true
		}
		location, _ := time.LoadLocation(connection.Policy.Timezone)
		oldest := asOf.In(location).AddDate(0, 0, -connection.Policy.HistoryDays)
		threadIDs := make([]string, 0, len(threads))
		for thread := range threads {
			threadIDs = append(threadIDs, thread)
		}
		sort.Strings(threadIDs)
		for _, thread := range threadIDs {
			messages, e := client.Thread(ctx, thread)
			if e != nil {
				state.Errors = append(state.Errors, "thread history unavailable: "+thread)
				continue
			}
			sort.SliceStable(messages, func(i, j int) bool {
				a, _ := strconv.ParseInt(messages[i].InternalDate, 10, 64)
				b, _ := strconv.ParseInt(messages[j].InternalDate, 10, 64)
				if a == b {
					return messages[i].ID < messages[j].ID
				}
				return a > b
			})
			for _, raw := range messages {
				if seen[SourceID(connection.ID, raw.ID)] {
					continue
				}
				received, e := strconv.ParseInt(raw.InternalDate, 10, 64)
				if e != nil {
					state.Errors = append(state.Errors, "thread source has unknown time: "+raw.ID)
					continue
				}
				at := time.UnixMilli(received)
				if at.After(asOf) || at.Before(oldest) {
					continue
				}
				if len(state.Sources) >= c.Config.Limits.MaxMessages {
					state.Errors = append(state.Errors, "reference history reached the configured source limit")
					break
				}
				normalized, e := Normalize(connection.ID, raw, mail.Reference, asOf, c.Config.Limits)
				if e != nil {
					state.Errors = append(state.Errors, "thread source normalization unavailable: "+raw.ID)
					continue
				}
				state.Sources = append(state.Sources, normalized)
				seen[normalized.Message.ID] = true
			}
			if err = persist(); err != nil {
				return a, err
			}
		}
	}
	snapshot := mail.Snapshot{Version: mail.Version, AsOf: a.AsOf, Timezone: connection.Policy.Timezone, Collection: mail.Collection{Status: "complete", Errors: append([]string{}, state.Errors...), Scope: fmt.Sprintf("Query: %s; batch limit: %d; thread history: %t within %d days; no separate attachment, image or calendar reads", connection.Policy.Query, connection.Policy.BatchSize, connection.Policy.ThreadHistory, connection.Policy.HistoryDays)}, Messages: []mail.Message{}, PriorInterpretations: []mail.Interpretation{}, Origin: &mail.Origin{Provider: "gmail", ConnectionID: connection.ID, AcquisitionID: a.ID, PolicyDigest: state.PolicyDigest}}
	for _, source := range state.Sources {
		snapshot.Messages = append(snapshot.Messages, source.Message)
	}
	if state.NextPageToken != "" {
		snapshot.Collection.Errors = append(snapshot.Collection.Errors, "additional mailbox pages remain; continue the acquisition explicitly")
	}
	if len(snapshot.Collection.Errors) > 0 {
		snapshot.Collection.Status = "partial"
	}
	encoded, _ := json.Marshal(snapshot)
	if _, err = mail.ParseSnapshot(encoded, c.Config.Limits); err != nil {
		return fail(err)
	}
	state.Snapshot = &snapshot
	a.Status = "collected"
	if snapshot.Collection.Status == "partial" {
		a.Status = "partial"
	}
	err = persist()
	return a, err
}

func (c Collector) Queue(ctx context.Context, id string) (store.Job, error) {
	a, state, err := c.state(ctx, id)
	if err != nil {
		return store.Job{}, err
	}
	if state.Snapshot == nil {
		return store.Job{}, errors.New("acquisition has no completed snapshot; resume collection first")
	}
	existing, err := c.Store.JobForAcquisition(ctx, id)
	if err == nil {
		if a.JobID != existing.ID {
			a.JobID = existing.ID
			_, err = c.Store.SaveAcquisition(ctx, a, state, c.Config.Limits.MaxEvidenceBytes)
		}
		return existing, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return store.Job{}, err
	}
	input, err := json.Marshal(state.Snapshot)
	if err != nil {
		return store.Job{}, err
	}
	job, err := c.Store.Submit(ctx, mail.Workgroup, input, map[string]any{"origin": "gmail", "connection_id": a.ConnectionID, "acquisition_id": a.ID}, c.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Job{}, err
	}
	a.JobID = job.ID
	_, err = c.Store.SaveAcquisition(ctx, a, state, c.Config.Limits.MaxEvidenceBytes)
	return job, err
}
