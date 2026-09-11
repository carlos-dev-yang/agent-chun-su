package jira

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/secrets"
	"chunsu/internal/store"
)

const (
	Collecting = "collecting"
	Collected  = "collected"
	Failed     = "failed"
)

// ByteBudgets are pinned with the acquisition, independently of mail limits.
type ByteBudgets struct {
	Response int64 `json:"response"`
	Text     int64 `json:"text"`
	Evidence int64 `json:"evidence"`
}

func budgets(l config.Limits) ByteBudgets {
	return ByteBudgets{Response: l.MaxArtifactBytes, Text: l.MaxSourceBytes, Evidence: l.MaxEvidenceBytes}
}
func (b ByteBudgets) limits() config.Limits {
	return config.Limits{MaxArtifactBytes: b.Response, MaxSourceBytes: b.Text, MaxEvidenceBytes: b.Evidence}
}

type PreservedPage struct {
	Cursor     string    `json:"cursor"`
	CapturedAt string    `json:"captured_at"`
	Response   Reference `json:"response"`
	NextCursor string    `json:"next_cursor"`
	Complete   bool      `json:"complete"`
	Processed  bool      `json:"processed"`
	Rows       int       `json:"rows"`
}

type State struct {
	Version           int             `json:"version"`
	Provider          string          `json:"provider"`
	ID                string          `json:"id"`
	Reader            string          `json:"reader"`
	Format            string          `json:"format"`
	Synthetic         bool            `json:"synthetic"`
	ReprocessOf       string          `json:"reprocess_of,omitempty"`
	Policy            Policy          `json:"policy"`
	PolicyDigest      string          `json:"policy_digest"`
	Normalizer        string          `json:"normalizer"`
	Budgets           ByteBudgets     `json:"byte_budgets"`
	Source            Reference       `json:"source"`
	Pages             []PreservedPage `json:"pages"`
	Snapshot          Reference       `json:"snapshot"`
	Cursor            string          `json:"cursor"`
	Rows              int             `json:"rows"`
	Status            string          `json:"status"`
	StopReason        string          `json:"stop_reason"`
	RetrievalComplete bool            `json:"retrieval_complete"`
}

type Record struct {
	Acquisition store.Acquisition `json:"acquisition"`
	State       State             `json:"state"`
	Snapshot    Snapshot          `json:"snapshot"`
}

type Collector struct {
	Store  *store.Store
	Config config.Config
	// ReservedID is supplied by a durable scheduler tick. It is never derived
	// from provider data and prevents a post-reservation collection from making
	// an unrelated acquisition identity.
	ReservedID string
}

type liveSource struct {
	Version            int          `json:"version"`
	Reader             string       `json:"reader"`
	ConnectionID       string       `json:"connection_id"`
	SiteHost           string       `json:"site_host"`
	CloudID            string       `json:"cloud_id"`
	PolicyDigest       string       `json:"policy_digest"`
	ReportPolicy       ReportPolicy `json:"report_policy"`
	ReportPolicyDigest string       `json:"report_policy_digest"`
	AsOfDate           string       `json:"as_of_date"`
}

func (c Collector) nextID() (string, error) {
	if c.ReservedID == "" {
		return files.ID(), nil
	}
	if !files.ValidID(c.ReservedID) {
		return "", errors.New("invalid reserved Jira acquisition ID")
	}
	return c.ReservedID, nil
}
func (p Profile) collectorPolicy() Policy {
	return Policy{Version: Version, ConnectionID: p.ID, ProjectKeys: []string{p.ProjectKey}, Subject: Identity{ID: p.SubjectAccountID, DisplayName: p.ExpectedAccount}, Selection: "assigned", SprintIDs: []string{}, Timezone: p.Timezone, MaxIssues: p.Policy.MaxIssues, MaxPages: p.Policy.MaxPages, MaxContextEntries: 1, Mapping: Mapping{Version: Version, IdentityField: "accountId", StartField: p.StartField}}
}
func (c Collector) liveSource(id string, p Profile, policy Policy, asOfDate string) (Reference, error) {
	reportPolicy := p.ReportPolicy()
	if _, err := time.Parse(time.DateOnly, asOfDate); err != nil {
		return Reference{}, errors.New("invalid pinned Jira report date")
	}
	b, err := json.Marshal(liveSource{Version: Version, Reader: LiveReaderKind, ConnectionID: p.ID, SiteHost: p.SiteHost, CloudID: p.CloudID, PolicyDigest: digest(policy), ReportPolicy: reportPolicy, ReportPolicyDigest: ReportPolicyDigest(reportPolicy), AsOfDate: asOfDate})
	if err != nil {
		return Reference{}, err
	}
	return c.write(id, "live-request", b, c.Config.Limits.MaxEvidenceBytes)
}
func (c Collector) readLiveSource(id string, ref Reference, policy Policy) (liveSource, error) {
	b, err := c.read(id, ref, c.Config.Limits.MaxEvidenceBytes)
	if err != nil {
		return liveSource{}, err
	}
	var source liveSource
	if err = decode(b, &source); err != nil {
		return source, err
	}
	if source.Version != Version || source.Reader != LiveReaderKind || source.ConnectionID != policy.ConnectionID || !plainText(source.SiteHost, 255) || !cloudID(source.CloudID) || source.PolicyDigest != digest(policy) || source.ReportPolicyDigest != ReportPolicyDigest(source.ReportPolicy) {
		return source, errors.New("live Jira request binding mismatch")
	}
	if err = source.ReportPolicy.Validate(); err != nil {
		return source, err
	}
	if _, err = time.Parse(time.DateOnly, source.AsOfDate); err != nil || len(source.ReportPolicy.ProjectKeys) != 1 || len(source.ReportPolicy.BoardIDs) != 1 || source.ReportPolicy.ConnectionID != source.ConnectionID || source.ReportPolicy.ProjectKeys[0] != policy.ProjectKeys[0] || source.ReportPolicy.SubjectAccountID != policy.Subject.ID {
		return source, errors.New("live Jira report scope binding mismatch")
	}
	return source, nil
}

func (c Collector) write(id, kind string, data []byte, limit int64) (Reference, error) {
	ref := Reference{Path: filepath.ToSlash(filepath.Join("state", "acquisitions", id, kind+"-"+files.ID()+".json")), Digest: files.Digest(data), Bytes: int64(len(data))}
	if ref.Bytes > limit {
		return Reference{}, errors.New("Jira evidence exceeds the pinned byte budget")
	}
	return ref, files.Write(c.Store.Root, ref.Path, data, false)
}

func (c Collector) read(id string, ref Reference, limit int64) ([]byte, error) {
	prefix := filepath.ToSlash(filepath.Join("state", "acquisitions", id)) + "/"
	if !files.ValidID(id) || !strings.HasPrefix(ref.Path, prefix) || filepath.ToSlash(filepath.Clean(ref.Path)) != ref.Path || !files.ValidDigest(ref.Digest) || ref.Bytes < 0 {
		return nil, errors.New("invalid Jira evidence reference")
	}
	data, err := files.Read(c.Store.Root, ref.Path, limit)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != ref.Bytes || files.Digest(data) != ref.Digest {
		return nil, errors.New("Jira evidence integrity mismatch")
	}
	return data, nil
}

func unique(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func (c Collector) checkpoint(ctx context.Context, r *Record) error {
	r.Snapshot.Gaps = unique(r.Snapshot.Gaps)
	r.Snapshot.Status = Partial
	if r.State.RetrievalComplete && len(r.Snapshot.Gaps) == 0 {
		r.Snapshot.Status = Complete
	}
	if err := validateSnapshot(r.Snapshot, r.State.Policy, r.State.Budgets.limits()); err != nil {
		return err
	}
	data, err := json.Marshal(r.Snapshot)
	if err != nil {
		return err
	}
	r.State.Snapshot, err = c.write(r.State.ID, "snapshot", data, r.State.Budgets.Evidence)
	if err != nil {
		return err
	}
	r.Acquisition.Status = r.State.Status
	r.Acquisition, err = c.Store.SaveAcquisition(ctx, r.Acquisition, r.State, r.State.Budgets.Evidence)
	return err
}

func (c Collector) CollectSaved(ctx context.Context, data []byte) (Record, error) {
	return c.beginSaved(ctx, data, nil, "")
}

// CollectLive reads only a verified, active Cloud profile. It preserves every
// raw provider page before normalization and stores a token-free request binding.
func (c Collector) CollectLive(ctx context.Context, profile Profile) (Record, error) {
	if err := profile.Validate(c.Config); err != nil {
		return Record{}, err
	}
	if !profile.Active {
		return Record{}, errors.New("Jira connection is disabled or has not been verified")
	}
	policy := profile.collectorPolicy()
	if err := policy.Validate(); err != nil {
		return Record{}, err
	}
	id, err := c.nextID()
	if err != nil {
		return Record{}, err
	}
	zone, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return Record{}, err
	}
	asOfDate := time.Now().In(zone).Format(time.DateOnly)
	source, err := c.liveSource(id, profile, policy, asOfDate)
	if err != nil {
		return Record{}, err
	}
	r := Record{Acquisition: store.Acquisition{ID: id, ConnectionID: profile.ID, Status: Collecting}, State: State{Version: Version, Provider: Provider, ID: id, Reader: LiveReaderKind, Format: CloudV3, Synthetic: false, Policy: policy, PolicyDigest: digest(policy), Normalizer: NormalizerVersion, Budgets: budgets(c.Config.Limits), Source: source, Pages: []PreservedPage{}, Status: Collecting}, Snapshot: Snapshot{AcquisitionID: id, Version: Version, Provider: Provider, Format: CloudV3, Synthetic: false, ConnectionID: profile.ID, PolicyDigest: digest(policy), Normalizer: NormalizerVersion, Timezone: profile.Timezone, Status: Partial, Issues: []Issue{}, Dispositions: []Disposition{}, Gaps: []string{}}}
	if err = c.checkpoint(ctx, &r); err != nil {
		return r, err
	}
	k, err := secrets.OpenFor(profile.Secret.Store)
	if err != nil {
		return c.fail(ctx, r, "keychain_unavailable", err)
	}
	token, err := k.GetExternal(ctx, profile.Secret.Service, profile.Secret.Account)
	if err != nil {
		return c.fail(ctx, r, "credential_unavailable", err)
	}
	reader, err := NewLiveReader(profile, token, asOfDate, c.Config.Limits)
	if err != nil {
		return c.fail(ctx, r, "reader_unavailable", err)
	}
	return c.run(ctx, r, reader)
}

func (c Collector) ResumeLive(ctx context.Context, profile Profile, id string) (Record, error) {
	if err := profile.Validate(c.Config); err != nil || !profile.Active {
		return Record{}, errors.New("Jira connection is disabled or invalid")
	}
	r, err := c.Load(ctx, id)
	if err != nil {
		return r, err
	}
	source, err := c.readLiveSource(id, r.State.Source, r.State.Policy)
	if err != nil {
		return r, err
	}
	if r.State.Reader != LiveReaderKind || r.State.Synthetic || profile.ID != r.State.Policy.ConnectionID || profile.SiteHost != source.SiteHost || profile.CloudID != source.CloudID || digest(profile.collectorPolicy()) != r.State.PolicyDigest || profile.ReportPolicyDigest() != source.ReportPolicyDigest {
		return r, errors.New("live Jira resume profile or policy differs from the pinned acquisition")
	}
	if r.State.RetrievalComplete {
		return r, nil
	}
	if r.Acquisition.NotBefore > time.Now().UnixMilli() {
		return r, errors.New("provider retry is not yet eligible")
	}
	if r.State.Rows >= r.State.Policy.MaxIssues || r.State.StopReason == "issue_budget_exhausted" || r.State.StopReason == "page_budget_reached" || r.State.StopReason == "continuation_unavailable" {
		return r, errors.New("live Jira acquisition reached a pinned collection bound; start a new acquisition")
	}
	for _, page := range r.State.Pages {
		if page.Processed && page.Complete {
			return r, errors.New("live Jira acquisition has no continuation; start a new acquisition")
		}
	}
	k, err := secrets.OpenFor(profile.Secret.Store)
	if err != nil {
		return r, err
	}
	token, err := k.GetExternal(ctx, profile.Secret.Service, profile.Secret.Account)
	if err != nil {
		return r, err
	}
	reader, err := NewLiveReader(profile, token, source.AsOfDate, c.Config.Limits)
	if err != nil {
		return r, err
	}
	return c.run(ctx, r, reader)
}

func (c Collector) beginSaved(ctx context.Context, data []byte, mapping *Mapping, reprocessOf string) (Record, error) {
	reader, err := NewSavedReader(data, c.Config.Limits)
	if err != nil {
		return Record{}, err
	}
	p := reader.Input.Policy
	if mapping != nil {
		p.Mapping = *mapping
		if err = p.Validate(); err != nil {
			return Record{}, err
		}
	}
	id, err := c.nextID()
	if err != nil {
		return Record{}, err
	}
	source, err := c.write(id, "source", data, c.Config.Limits.MaxEvidenceBytes)
	if err != nil {
		return Record{}, err
	}
	r := Record{
		Acquisition: store.Acquisition{ID: id, ConnectionID: p.ConnectionID, Status: Collecting},
		State:       State{Version: Version, Provider: Provider, ID: id, Reader: SavedReaderKind, Format: reader.Input.Format, Synthetic: reader.Input.Synthetic, ReprocessOf: reprocessOf, Policy: p, PolicyDigest: digest(p), Normalizer: NormalizerVersion, Budgets: budgets(c.Config.Limits), Source: source, Pages: []PreservedPage{}, Status: Collecting},
		Snapshot:    Snapshot{AcquisitionID: id, Version: Version, Provider: Provider, Format: reader.Input.Format, Synthetic: reader.Input.Synthetic, ConnectionID: p.ConnectionID, PolicyDigest: digest(p), Normalizer: NormalizerVersion, Timezone: p.Timezone, Status: Partial, Issues: []Issue{}, Dispositions: []Disposition{}, Gaps: []string{}},
	}
	if err = c.checkpoint(ctx, &r); err != nil {
		return r, err
	}
	return c.run(ctx, r, reader)
}

// Load verifies the DB checkpoint, every referenced file, the original reader
// input, cursor progression and normalized provenance before exposing a record.
func (c Collector) Load(ctx context.Context, id string) (Record, error) {
	r := Record{}
	if !files.ValidID(id) {
		return r, errors.New("invalid Jira acquisition ID")
	}
	a, err := c.Store.Acquisition(ctx, id)
	if err != nil {
		return r, err
	}
	data, err := c.Store.ReadAcquisition(a, c.Config.Limits.MaxEvidenceBytes)
	if err != nil {
		return r, err
	}
	var state State
	if err = decode(data, &state); err != nil {
		return r, errors.New("acquisition is not a supported Jira checkpoint")
	}
	if state.Version != Version || state.Provider != Provider || state.ID != a.ID || state.Policy.ConnectionID != a.ConnectionID || state.PolicyDigest != digest(state.Policy) || state.Normalizer != NormalizerVersion || (state.Reader != SavedReaderKind && state.Reader != LiveReaderKind) || state.Status != a.Status || a.ParentID != "" || (state.ReprocessOf != "" && !files.ValidID(state.ReprocessOf)) || (state.Reader == LiveReaderKind && (state.Synthetic || state.ReprocessOf != "")) {
		return r, errors.New("Jira acquisition binding mismatch")
	}
	if err = state.Policy.Validate(); err != nil {
		return r, err
	}
	// Lowering local limits never silently increases them on replay.
	if state.Budgets.Response <= 0 || state.Budgets.Text <= 0 || state.Budgets.Evidence <= 0 || state.Budgets.Response > c.Config.Limits.MaxArtifactBytes || state.Budgets.Text > c.Config.Limits.MaxSourceBytes || state.Budgets.Evidence > c.Config.Limits.MaxEvidenceBytes {
		return r, errors.New("pinned Jira byte budgets are invalid or exceed current host limits")
	}
	var saved *SavedReader
	if state.Reader == SavedReaderKind {
		source, e := c.read(id, state.Source, state.Budgets.Evidence)
		if e != nil {
			return r, e
		}
		saved, e = NewSavedReader(source, state.Budgets.limits())
		if e != nil {
			return r, e
		}
		expected := saved.Input.Policy
		if state.ReprocessOf != "" {
			expected.Mapping = state.Policy.Mapping
		}
		if digest(expected) != state.PolicyDigest || state.Format != saved.Input.Format || state.Synthetic != saved.Input.Synthetic {
			return r, errors.New("preserved Jira source/policy mismatch")
		}
	} else if _, err = c.readLiveSource(id, state.Source, state.Policy); err != nil {
		return r, err
	}
	data, err = c.read(id, state.Snapshot, state.Budgets.Evidence)
	if err != nil {
		return r, err
	}
	var snapshot Snapshot
	if err = decode(data, &snapshot); err != nil {
		return r, err
	}
	if err = validateSnapshot(snapshot, state.Policy, state.Budgets.limits()); err != nil {
		return r, err
	}
	if snapshot.AcquisitionID != id || snapshot.Format != state.Format || snapshot.Synthetic != state.Synthetic {
		return r, errors.New("Jira snapshot origin mismatch")
	}
	cursor := ""
	rows := 0
	rawBytes := int64(0)
	complete := false
	seen := map[string]bool{}
	evidence := map[string]PreservedPage{}
	for index, page := range state.Pages {
		if complete || seen[page.Cursor] || page.Cursor != cursor || (!page.Processed && index != len(state.Pages)-1) || page.Rows < 0 {
			return r, errors.New("invalid Jira checkpoint progress")
		}
		seen[page.Cursor] = true
		body, e := c.read(id, page.Response, state.Budgets.Response)
		if e != nil {
			return r, e
		}
		rawBytes += int64(len(body))
		if rawBytes > state.Budgets.Evidence {
			return r, errors.New("Jira raw evidence exceeds pinned budget")
		}
		original := Page{Body: body, CapturedAt: page.CapturedAt, NextCursor: page.NextCursor, Complete: page.Complete}
		if saved != nil {
			original, e = saved.ReadPage(ctx, Request{Policy: state.Policy, Cursor: page.Cursor})
			if e != nil {
				return r, e
			}
		} else {
			original.NextCursor, original.Complete, e = pageContinuation(body, state.Format)
			if e != nil {
				return r, e
			}
		}
		at, _ := timestamp(original.CapturedAt)
		if files.Digest(original.Body) != page.Response.Digest || page.CapturedAt != at || page.NextCursor != original.NextCursor || page.Complete != original.Complete {
			return r, errors.New("Jira page metadata mismatch")
		}
		if page.Processed {
			cursor = page.NextCursor
			rows += page.Rows
			complete = page.Complete
			evidence[page.Response.Path] = page
		}
	}
	if rows != state.Rows || rows > state.Policy.MaxIssues || cursor != state.Cursor || (state.RetrievalComplete && (!complete || contains(snapshot.Gaps, "issue_budget_exhausted"))) || len(snapshot.Dispositions) != rows {
		return r, errors.New("Jira checkpoint totals mismatch")
	}
	if state.Status != Collecting && state.Status != Collected && state.Status != Partial && state.Status != Failed && state.Status != store.RetryWait {
		return r, errors.New("invalid Jira acquisition status")
	}
	if (state.Status == Collected) != state.RetrievalComplete || (snapshot.Status == Complete) != (state.RetrievalComplete && len(snapshot.Gaps) == 0) {
		return r, errors.New("Jira completion state mismatch")
	}
	checkEvidence := func(p Provenance) bool {
		page, ok := evidence[p.Response.Path]
		if !ok || p.Response != page.Response || p.CapturedAt != page.CapturedAt {
			return false
		}
		if !strings.HasPrefix(p.Pointer, issuePointerPrefix) {
			return false
		}
		index, err := strconv.Atoi(strings.TrimPrefix(p.Pointer, issuePointerPrefix))
		return err == nil && index >= 0 && index < page.Rows && p.Pointer == issuePointerPrefix+strconv.Itoa(index)
	}
	for _, i := range snapshot.Issues {
		for _, p := range i.Evidence {
			if !checkEvidence(p) {
				return r, errors.New("invalid Jira issue provenance")
			}
		}
	}
	for _, d := range snapshot.Dispositions {
		if !checkEvidence(d.Evidence) {
			return r, errors.New("invalid Jira disposition provenance")
		}
	}
	return Record{Acquisition: a, State: state, Snapshot: snapshot}, nil
}

func (c Collector) Resume(ctx context.Context, id string) (Record, error) {
	r, err := c.Load(ctx, id)
	if err != nil {
		return r, err
	}
	if r.State.Reader != SavedReaderKind {
		return r, errors.New("live Jira acquisitions require resume-live with the verified connection")
	}
	if r.State.RetrievalComplete {
		return r, nil
	}
	if r.State.Rows >= r.State.Policy.MaxIssues {
		return r, errors.New("pinned issue budget is exhausted; an explicit new acquisition is required")
	}
	if len(r.State.Pages) > 0 && r.State.Pages[len(r.State.Pages)-1].Processed && r.State.Cursor == "" {
		return r, errors.New("source has no continuation; start a new acquisition with additional saved evidence")
	}
	data, err := c.read(id, r.State.Source, r.State.Budgets.Evidence)
	if err != nil {
		return r, err
	}
	reader, err := NewSavedReader(data, r.State.Budgets.limits())
	if err != nil {
		return r, err
	}
	return c.run(ctx, r, reader)
}

func (c Collector) Renormalize(ctx context.Context, id string, mapping Mapping) (Record, error) {
	if err := mapping.Validate(); err != nil {
		return Record{}, err
	}
	r, err := c.Load(ctx, id)
	if err != nil {
		return r, err
	}
	if r.State.Reader != SavedReaderKind {
		return r, errors.New("live Jira acquisitions cannot be renormalized as saved input")
	}
	data, err := c.read(id, r.State.Source, r.State.Budgets.Evidence)
	if err != nil {
		return r, err
	}
	return c.beginSaved(ctx, data, &mapping, id)
}

func (c Collector) Source(ctx context.Context, id string, pageIndex int) ([]byte, error) {
	r, err := c.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if pageIndex < 0 || pageIndex >= len(r.State.Pages) {
		return nil, errors.New("page index is outside the preserved acquisition")
	}
	return c.read(id, r.State.Pages[pageIndex].Response, r.State.Budgets.Response)
}

// Queue creates at most one jira-report job for an available live acquisition.
// The submitted input is immutable, contains no Keychain reference, and pins
// the report policy independently from legacy collector normalization policy.
func (c Collector) Queue(ctx context.Context, id string) (store.Job, error) {
	r, err := c.Load(ctx, id)
	if err != nil {
		return store.Job{}, err
	}
	if r.State.Reader != LiveReaderKind || r.State.Synthetic || (r.State.Status != Collected && r.State.Status != Partial) {
		return store.Job{}, errors.New("Jira acquisition is not an available live snapshot")
	}
	source, err := c.readLiveSource(id, r.State.Source, r.State.Policy)
	if err != nil {
		return store.Job{}, err
	}
	profile, err := ReadProfile(c.Store.Root, r.State.Policy.ConnectionID, c.Config)
	if err != nil {
		return store.Job{}, err
	}
	if !profile.Active || profile.ID != r.State.Policy.ConnectionID || profile.SiteHost != source.SiteHost || profile.CloudID != source.CloudID || profile.ReportPolicyDigest() != source.ReportPolicyDigest {
		return store.Job{}, errors.New("Jira report profile differs from the pinned live acquisition")
	}
	if existing, e := c.Store.JobForAcquisition(ctx, id); e == nil {
		if r.Acquisition.JobID != existing.ID {
			r.Acquisition.JobID = existing.ID
			_, e = c.Store.SaveAcquisition(ctx, r.Acquisition, r.State, c.Config.Limits.MaxEvidenceBytes)
		}
		return existing, e
	} else if !errors.Is(e, sql.ErrNoRows) {
		return store.Job{}, e
	}
	input := ReportInput{Version: ReportInputVersion, Policy: source.ReportPolicy, ReportPolicyDigest: source.ReportPolicyDigest, CollectionPolicyDigest: r.Snapshot.PolicyDigest, Snapshot: r.Snapshot, AsOfDate: source.AsOfDate, BoardID: source.ReportPolicy.BoardIDs[0], SiteHost: source.SiteHost, TodoStatusID: source.ReportPolicy.TodoStatusID}
	b, err := json.Marshal(input)
	if err != nil {
		return store.Job{}, err
	}
	if _, err = ParseReportInput(b); err != nil {
		return store.Job{}, err
	}
	job, err := c.Store.Submit(ctx, "jira-report", b, map[string]any{"origin": "jira-cloud", "connection_id": r.Acquisition.ConnectionID, "acquisition_id": r.Acquisition.ID, "report_policy_digest": input.ReportPolicyDigest}, c.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Job{}, err
	}
	r.Acquisition.JobID = job.ID
	_, err = c.Store.SaveAcquisition(ctx, r.Acquisition, r.State, c.Config.Limits.MaxEvidenceBytes)
	return job, err
}

func (c Collector) fail(ctx context.Context, r Record, reason string, cause error) (Record, error) {
	r.State.Status = Failed
	r.State.StopReason = reason
	var api *LiveAPIError
	if reason == "reader_failed" && errors.As(cause, &api) && transient(api.Kind) {
		r.State.Status = store.RetryWait
		r.State.StopReason = "retry_wait"
		r.Acquisition.NotBefore = time.Now().Add(time.Duration(c.Config.Limits.RetryDelaySeconds) * time.Second).UnixMilli()
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(c.Config.Limits.TimeoutSeconds)*time.Second)
	defer cancel()
	if err := c.checkpoint(saveCtx, &r); err != nil {
		return r, fmt.Errorf("Jira acquisition %s stopped (%s); checkpoint failed: %w", r.State.ID, reason, err)
	}
	return r, fmt.Errorf("Jira acquisition %s stopped (%s): %w", r.State.ID, reason, cause)
}

// run depends on Reader, not SavedReader. A later host-owned HTTP reader can use
// the same preservation/normalization loop after its connection is reviewed.
func (c Collector) run(ctx context.Context, r Record, reader Reader) (Record, error) {
	r.State.Status = Collecting
	r.State.StopReason = ""
	r.Acquisition.NotBefore = 0
	pageLimit := r.State.Policy.MaxPages
	if r.State.Reader == LiveReaderKind {
		for _, page := range r.State.Pages {
			if page.Processed {
				pageLimit--
			}
		}
		if pageLimit <= 0 {
			return c.fail(ctx, r, "page_budget_exhausted", errors.New("live Jira acquisition reached its pinned page budget"))
		}
	}
	for processed := 0; processed < pageLimit; processed++ {
		if err := ctx.Err(); err != nil {
			return c.fail(ctx, r, "interrupted", err)
		}
		var page Page
		var preserved PreservedPage
		pending := len(r.State.Pages) > 0 && !r.State.Pages[len(r.State.Pages)-1].Processed
		if pending {
			preserved = r.State.Pages[len(r.State.Pages)-1]
			body, err := c.read(r.State.ID, preserved.Response, r.State.Budgets.Response)
			if err != nil {
				return c.fail(ctx, r, "evidence_unavailable", err)
			}
			page = Page{Body: body, CapturedAt: preserved.CapturedAt, NextCursor: preserved.NextCursor, Complete: preserved.Complete}
		} else {
			for _, prior := range r.State.Pages {
				if prior.Cursor == r.State.Cursor {
					return c.fail(ctx, r, "repeated_cursor", errors.New("reader continuation did not advance"))
				}
			}
			var err error
			page, err = reader.ReadPage(ctx, Request{Policy: r.State.Policy, Cursor: r.State.Cursor})
			if err != nil {
				return c.fail(ctx, r, "reader_failed", err)
			}
			at, err := timestamp(page.CapturedAt)
			if err != nil {
				return c.fail(ctx, r, "invalid_page", err)
			}
			if page.Complete && page.NextCursor != "" {
				return c.fail(ctx, r, "invalid_page", errors.New("completed page cannot have a continuation"))
			}
			rawBytes := int64(len(page.Body))
			for _, prior := range r.State.Pages {
				rawBytes += prior.Response.Bytes
			}
			if rawBytes > r.State.Budgets.Evidence {
				return c.fail(ctx, r, "evidence_budget_exhausted", errors.New("raw pages exceed pinned evidence budget"))
			}
			ref, err := c.write(r.State.ID, "response", page.Body, r.State.Budgets.Response)
			if err != nil {
				return c.fail(ctx, r, "preservation_failed", err)
			}
			preserved = PreservedPage{Cursor: r.State.Cursor, CapturedAt: at, Response: ref, NextCursor: page.NextCursor, Complete: page.Complete}
			r.State.Pages = append(r.State.Pages, preserved)
			if err = c.checkpoint(ctx, &r); err != nil {
				return r, err
			}
		}
		normalized, err := NormalizePage(page, r.State.Policy, preserved.Response, r.State.Policy.MaxIssues-r.State.Rows, r.State.Budgets.limits())
		if err != nil {
			return c.fail(ctx, r, "normalization_failed", err)
		}
		merge(&r.Snapshot, normalized)
		at, _ := timestamp(page.CapturedAt)
		if r.Snapshot.CapturedFrom == "" || earlier(at, r.Snapshot.CapturedFrom) {
			r.Snapshot.CapturedFrom = at
		}
		if r.Snapshot.CapturedThrough == "" || earlier(r.Snapshot.CapturedThrough, at) {
			r.Snapshot.CapturedThrough = at
		}
		r.State.Pages[len(r.State.Pages)-1].Processed = true
		r.State.Pages[len(r.State.Pages)-1].Rows = normalized.Rows
		r.State.Rows += normalized.Rows
		r.State.Cursor = page.NextCursor
		r.State.RetrievalComplete = page.Complete && !contains(r.Snapshot.Gaps, "issue_budget_exhausted")
		switch {
		case r.State.RetrievalComplete:
			r.State.Status = Collected
			r.State.StopReason = "end_of_source"
		case r.State.Rows >= r.State.Policy.MaxIssues:
			r.State.Status = Partial
			r.State.StopReason = "issue_budget_exhausted"
		case page.NextCursor == "":
			r.State.Status = Partial
			r.State.StopReason = "continuation_unavailable"
		case processed+1 >= pageLimit:
			r.State.Status = Partial
			r.State.StopReason = "page_budget_reached"
		}
		if err = c.checkpoint(ctx, &r); err != nil {
			return r, err
		}
		if r.State.Status != Collecting {
			return r, nil
		}
	}
	return r, nil
}

func earlier(a, b string) bool {
	left, _ := time.Parse(time.RFC3339Nano, a)
	right, _ := time.Parse(time.RFC3339Nano, b)
	return left.Before(right)
}
func factDigest(i Issue) string {
	i.Evidence = nil
	i.Limitations = nil
	i.ConflictingVersions = false
	return digest(i)
}
func merge(snapshot *Snapshot, page NormalizedPage) {
	indices := map[string]int{}
	for index, i := range snapshot.Issues {
		indices[i.ID] = index
	}
	byPointer := map[string]Issue{}
	for _, i := range page.Issues {
		byPointer[i.Evidence[0].Pointer] = i
	}
	for _, d := range page.Dispositions {
		if d.Outcome == "included" {
			i := byPointer[d.Evidence.Pointer]
			if index, exists := indices[i.ID]; exists {
				stored := &snapshot.Issues[index]
				d.Outcome = "duplicate"
				d.Reason = "same_normalized_facts"
				if factDigest(*stored) != factDigest(i) {
					stored.ConflictingVersions = true
					d.Outcome = "conflicting_duplicate"
					d.Reason = "first_observation_retained_with_all_sources"
					snapshot.Gaps = append(snapshot.Gaps, "conflicting_versions:"+i.ID)
				}
				stored.Evidence = append(stored.Evidence, i.Evidence...)
			} else {
				indices[i.ID] = len(snapshot.Issues)
				snapshot.Issues = append(snapshot.Issues, i)
			}
		}
		snapshot.Dispositions = append(snapshot.Dispositions, d)
	}
	snapshot.Gaps = append(snapshot.Gaps, page.Gaps...)
}
