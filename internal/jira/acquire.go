package jira

import (
	"context"
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
	id := files.ID()
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
	if state.Version != Version || state.Provider != Provider || state.ID != a.ID || state.Policy.ConnectionID != a.ConnectionID || state.PolicyDigest != digest(state.Policy) || state.Normalizer != NormalizerVersion || state.Reader != SavedReaderKind || state.Status != a.Status || a.JobID != "" || a.ParentID != "" || (state.ReprocessOf != "" && !files.ValidID(state.ReprocessOf)) {
		return r, errors.New("Jira acquisition binding mismatch")
	}
	if err = state.Policy.Validate(); err != nil {
		return r, err
	}
	// Lowering local limits never silently increases them on replay.
	if state.Budgets.Response <= 0 || state.Budgets.Text <= 0 || state.Budgets.Evidence <= 0 || state.Budgets.Response > c.Config.Limits.MaxArtifactBytes || state.Budgets.Text > c.Config.Limits.MaxSourceBytes || state.Budgets.Evidence > c.Config.Limits.MaxEvidenceBytes {
		return r, errors.New("pinned Jira byte budgets are invalid or exceed current host limits")
	}
	source, err := c.read(id, state.Source, state.Budgets.Evidence)
	if err != nil {
		return r, err
	}
	reader, err := NewSavedReader(source, state.Budgets.limits())
	if err != nil {
		return r, err
	}
	expected := reader.Input.Policy
	if state.ReprocessOf != "" {
		expected.Mapping = state.Policy.Mapping
	}
	if digest(expected) != state.PolicyDigest || state.Format != reader.Input.Format || state.Synthetic != reader.Input.Synthetic {
		return r, errors.New("preserved Jira source/policy mismatch")
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
		original, e := reader.ReadPage(ctx, Request{Policy: state.Policy, Cursor: page.Cursor})
		if e != nil {
			return r, e
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
	if state.Status != Collecting && state.Status != Collected && state.Status != Partial && state.Status != Failed {
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

func (c Collector) fail(ctx context.Context, r Record, reason string, cause error) (Record, error) {
	r.State.Status = Failed
	r.State.StopReason = reason
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
	for processed := 0; processed < r.State.Policy.MaxPages; processed++ {
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
		case processed+1 >= r.State.Policy.MaxPages:
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
