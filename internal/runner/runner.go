package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/gateway"
	"chunsu/internal/gmail"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/platform"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

type Runner struct {
	Store  *store.Store
	Config config.Config
	mu     sync.Mutex
	active string
	cancel context.CancelFunc
}
type Outcome struct {
	JobID      string `json:"job_id"`
	AttemptID  string `json:"attempt_id"`
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
	ReportPath string `json:"report_path,omitempty"`
}

func (r *Runner) Handle(ctx context.Context, req control.Request) (any, error) {
	switch req.Operation {
	case "queue":
		workgroupID := req.Workgroup
		if workgroupID == "" {
			workgroupID = mail.Workgroup
		}
		switch workgroupID {
		case mail.Workgroup:
			if _, err := mail.ParseSnapshot(req.Input, r.Config.Limits); err != nil {
				return nil, err
			}
		case "jira-report":
			if _, err := jira.ParseReportInput(req.Input); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("unsupported queue workgroup")
		}
		request := map[string]any{"origin": "saved", "admission": "management"}
		if req.SourceName != "" {
			request["source_name"] = req.SourceName
		}
		return r.Store.Submit(ctx, workgroupID, req.Input, request, r.Config.Limits.MaxArtifactBytes)
	case "cancel":
		r.mu.Lock()
		defer r.mu.Unlock()
		if err := r.Store.Cancel(ctx, req.JobID); err != nil {
			return nil, err
		}
		if r.active == req.JobID && r.cancel != nil {
			r.cancel()
		}
		return map[string]string{"job_id": req.JobID, "status": store.Cancelled}, nil
	case "retry", "resolve":
		attempts, err := r.Store.Attempts(ctx, req.JobID)
		if err != nil {
			return nil, err
		}
		if len(attempts) >= r.Config.Limits.MaxAttempts {
			return nil, errors.New("attempt budget exhausted; review the limit or queue a separate experiment")
		}
		if req.Operation == "resolve" && !mail.Nonempty(req.Answer) {
			return nil, errors.New("answer must not be empty")
		}
		if int64(len(req.Answer)) > r.Config.Limits.MaxSourceBytes {
			return nil, errors.New("answer exceeds source byte limit")
		}
		if err := r.Store.Resume(ctx, req.JobID, req.Answer); err != nil {
			return nil, err
		}
		return map[string]string{"job_id": req.JobID, "status": store.Queued}, nil
	case "status":
		r.mu.Lock()
		id := r.active
		r.mu.Unlock()
		paused, err := r.Store.Paused(ctx)
		return map[string]any{"active_job": id, "owner": "controller", "queue_paused": paused}, err
	case "pause", "unpause":
		r.mu.Lock()
		defer r.mu.Unlock()
		paused := req.Operation == "pause"
		if err := r.Store.SetPaused(ctx, paused); err != nil {
			return nil, err
		}
		return map[string]any{"queue_paused": paused, "active_job": r.active, "active_work": "allowed to finish; use cancel to stop a running job"}, nil
	default:
		return nil, errors.New("unsupported management operation")
	}
}

func (r *Runner) Run(ctx context.Context, jobID, candidate string) (out Outcome, runErr error) {
	out.JobID = jobID
	if r.Config.Executor.Kind == "" || r.Config.Executor.Path == "" {
		return out, errors.New("executor is not configured; use config set after selecting your executor account")
	}
	r.mu.Lock()
	paused, err := r.Store.Paused(ctx)
	if err != nil || paused {
		r.mu.Unlock()
		if err != nil {
			return out, err
		}
		out.Status = store.Queued
		return out, errors.New("queue is paused; use unpause after reviewing pending work")
	}
	if r.active != "" {
		r.mu.Unlock()
		return out, errors.New("another job is already running")
	}
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(r.Config.Limits.TimeoutSeconds)*time.Second)
	r.active = jobID
	r.cancel = cancel
	r.mu.Unlock()
	defer func() { cancel(); r.mu.Lock(); r.active = ""; r.cancel = nil; r.mu.Unlock() }()
	// Finalization must still be possible after a timeout or user cancellation.
	finishCtx := context.WithoutCancel(ctx)
	if _, err := Recover(ctx, r.Store, r.Config); err != nil {
		return out, err
	}
	j, err := r.Store.Job(ctx, jobID)
	if err != nil {
		return out, err
	}
	a, err := r.Store.StartAttempt(ctx, jobID, r.Config.Executor.Kind, r.Config.Limits.MaxAttempts)
	if err != nil {
		if errors.Is(err, store.ErrAttemptBudget) {
			out.Status = store.WaitingInput
			out.Diagnostic = err.Error()
		}
		return out, err
	}
	out.AttemptID = a.ID
	finish := func(status, diagnostic string, retry bool) error {
		current, e := r.Store.Job(finishCtx, jobID)
		if e != nil {
			return e
		}
		if current.CurrentAttempt != a.ID {
			return errors.New("stale attempt cannot finalize")
		}
		if current.Status == store.Cancelled {
			out.Status = store.Cancelled
			return nil
		}
		var next int64
		if retry && a.Ordinal < r.Config.Limits.MaxAttempts {
			status = store.RetryWait
			next = time.Now().Add(time.Duration(r.Config.Limits.RetryDelaySeconds) * time.Second).UnixMilli()
		}
		out.Status = status
		out.Diagnostic = diagnostic
		return r.Store.FinishAttempt(finishCtx, a, status, diagnostic, next)
	}
	if candidate == "" {
		var request struct {
			Candidate string `json:"candidate_digest"`
		}
		if e := json.Unmarshal(j.Request, &request); e == nil {
			candidate = request.Candidate
		}
	}
	p, err := workgroup.Prepare(ctx, r.Store, r.Config, j, a, candidate)
	if err != nil {
		e := finish(store.Failed, "package preparation: "+err.Error(), false)
		return out, errors.Join(err, e)
	}
	if p.Workgroup == "jira-report" && p.JiraSnapshot != nil && !p.JiraSnapshot.Snapshot.Synthetic {
		if e := VerifyJiraLiveProof(ctx, r.Store, r.Config, p); e != nil {
			e = finish(store.WaitingInput, e.Error(), false)
			return out, errors.Join(errors.New("live Jira disclosure proof is no longer valid"), e)
		}
	}
	if p.Snapshot.Origin != nil {
		connection, e := gmail.LoadConnection(r.Store.Root, p.Snapshot.Origin.ConnectionID, r.Config)
		if e != nil || gmail.PolicyDigest(connection.Policy) != p.Snapshot.Origin.PolicyDigest {
			cause := errors.New("live connection is disabled, unavailable or has a different policy; review the original acquisition before retry")
			e = finish(store.WaitingInput, cause.Error(), false)
			return out, errors.Join(cause, e)
		}
	}
	if p.JiraSnapshot != nil && !p.JiraSnapshot.Snapshot.Synthetic {
		profile, e := jira.LoadProfile(r.Store.Root, p.JiraSnapshot.Policy.ConnectionID, r.Config)
		if e != nil || !profile.Active || profile.SiteHost != p.JiraSnapshot.SiteHost || profile.ReportPolicyDigest() != p.JiraSnapshot.ReportPolicyDigest || p.Executor.LiveJiraPolicyDigest != p.JiraSnapshot.ReportPolicyDigest {
			cause := errors.New("live Jira profile is disabled, unavailable or has a different reviewed report policy; review the original acquisition before retry")
			e = finish(store.WaitingInput, cause.Error(), false)
			return out, errors.Join(cause, e)
		}
	}
	generated, execErr := executor.Run(attemptCtx, r.Store.Root, p)
	r.mu.Lock()
	defer r.mu.Unlock()
	metadata, _ := json.Marshal(generated)
	if _, err = r.Store.SaveArtifact(finishCtx, jobID, a.ID, "executor_result", metadata, r.Config.Limits.MaxArtifactBytes); err != nil {
		return out, err
	}
	if len(generated.Final) > 0 {
		if _, err = r.Store.SaveArtifact(finishCtx, jobID, a.ID, "raw_result", generated.Final, r.Config.Limits.MaxArtifactBytes); err != nil {
			return out, err
		}
	}
	if generated.Outcome == "orphaned" {
		e := finish(store.WaitingInput, execErr.Error(), false)
		return out, errors.Join(execErr, e)
	}
	observed, err := r.collectLookups(finishCtx, j, a)
	if err != nil {
		e := finish(store.WaitingInput, "lookup evidence collection failed: "+err.Error(), false)
		return out, errors.Join(err, e)
	}
	if executor.HasCapabilityViolation(generated.ObservedTools, p.Workgroup) {
		cause := errors.New(executor.CapabilityViolation)
		e := finish(store.Failed, executor.CapabilityViolation, false)
		return out, errors.Join(cause, e)
	}
	if execErr != nil {
		status := store.Failed
		retry := true
		if errors.Is(execErr, context.Canceled) {
			status = store.Cancelled
			retry = false
		}
		if generated.Outcome == "not_started" {
			status = store.WaitingInput
			retry = false
		}
		if generated.Outcome == "waiting_auth" {
			status = store.WaitingAuth
			retry = false
		}
		e := finish(status, execErr.Error(), retry)
		return out, errors.Join(execErr, e)
	}
	if err = r.Store.AddEvent(finishCtx, jobID, a.ID, "result.generated", map[string]any{"bytes": len(generated.Final)}); err != nil {
		return out, err
	}
	if p.Workgroup == "jira-report" {
		jiraOut, jiraErr := r.completeJiraResult(finishCtx, out, finish, j, a, p, generated.Final, observed)
		if jiraOut.Status == "" {
			jiraOut.Status, jiraOut.Diagnostic = out.Status, out.Diagnostic
		}
		return jiraOut, jiraErr
	}
	report, validation, validationErr := mail.ValidateReport(generated.Final, p.Bundle.Schema, p.Snapshot, p.Mode, observed)
	if validationErr != nil {
		validation.Gaps = append(validation.Gaps, validationErr.Error())
	}
	b, _ := json.MarshalIndent(validation, "", "  ")
	if _, err = r.Store.SaveArtifact(finishCtx, jobID, a.ID, "validation", b, r.Config.Limits.MaxArtifactBytes); err != nil {
		return out, err
	}
	if missingRequiredSourceLookups(p.Snapshot, observed) {
		cause := errors.New(requiredSourceLookupsMissing)
		e := finish(store.Failed, requiredSourceLookupsMissing, false)
		return out, errors.Join(cause, e)
	}
	if validationErr != nil {
		e := finish(store.Failed, "result contract: "+validationErr.Error(), true)
		return out, errors.Join(validationErr, e)
	}
	art, err := r.savePresentation(finishCtx, jobID, a.ID, report, validation, p.Snapshot)
	if err != nil {
		e := finish(store.WaitingInput, store.PublicationRequired, false)
		return out, errors.Join(err, e)
	}
	out.ReportPath = filepath.Join(r.Store.Root, art.Path)
	if err = r.Store.AddEvent(finishCtx, jobID, a.ID, "report.available", map[string]string{"artifact_id": art.ID, "availability": "local", "acknowledgment": "unknown"}); err != nil {
		return out, err
	}
	if err = finish(validation.OperationalStatus, "", false); err != nil {
		return out, err
	}
	if p.Snapshot.Origin != nil && !j.IsExperiment() && (out.Status == store.Completed || out.Status == store.Partial) {
		if err = r.Store.RecordCoverage(finishCtx, p.Snapshot.Origin.ConnectionID, jobID, gmail.CoverageForReport(p.Snapshot, report)); err != nil {
			_ = r.Store.AddEvent(finishCtx, jobID, a.ID, "coverage.pending", map[string]string{"reason": "source coverage update failed after report publication"})
			return out, err
		}
	}
	return out, nil
}

const requiredSourceLookupsMissing = "required_source_lookups_missing"

// VerifyJiraLiveProof rechecks the approval evidence immediately before a
// live executor invocation. The synthetic proof must use this exact selected
// bundle; an inactive candidate remains eligible for comparison without
// changing the human-owned active workgroup pointer.
func VerifyJiraLiveProof(ctx context.Context, s *store.Store, c config.Config, p workgroup.Package) error {
	if !c.Executor.LiveJiraApproved || c.Executor.LiveJiraValidationJobID == "" || p.JiraSnapshot == nil || p.JiraSnapshot.Snapshot.Synthetic {
		return errors.New("live Jira disclosure is not approved with a synthetic proof")
	}
	if c.Executor.LiveJiraPolicyDigest != p.JiraSnapshot.ReportPolicyDigest {
		return errors.New("live Jira report policy differs from the approved policy digest")
	}
	bundle, err := workgroup.LoadFor(s.Root, "jira-report", p.WorkgroupDigest, c.Limits.MaxArtifactBytes)
	if err != nil {
		return err
	}
	skill, err := bundle.SelectedSkill()
	if err != nil {
		return err
	}
	if p.Skill.Name != skill.Name || p.Skill.Description != skill.Description || p.Skill.Digest != files.Digest([]byte(skill.Markdown)) {
		return errors.New("live Jira package does not use its selected Jira Skill")
	}
	return verifyJiraSyntheticProof(ctx, s, c, c.Executor.LiveJiraValidationJobID, p.WorkgroupDigest, skill)
}

// VerifyJiraSyntheticProof verifies the durable evidence needed to approve
// live Jira disclosure for the current executor and the proof attempt's
// preserved selected Jira Skill. It never treats an empty synthetic snapshot
// as a boundary demonstration.
func VerifyJiraSyntheticProof(ctx context.Context, s *store.Store, c config.Config, jobID string) error {
	return verifyJiraSyntheticProof(ctx, s, c, jobID, "", workgroup.Skill{})
}

func verifyJiraSyntheticProof(ctx context.Context, s *store.Store, c config.Config, jobID, requiredDigest string, requiredSkill workgroup.Skill) error {
	if jobID == "" || !files.ValidID(jobID) {
		return errors.New("Jira live disclosure proof requires a valid synthetic job ID")
	}
	if c.Executor.Kind == "" || c.Executor.Path == "" {
		return errors.New("select the executor before approving live Jira disclosure")
	}
	j, err := s.Job(ctx, jobID)
	if err != nil {
		return fmt.Errorf("read Jira disclosure proof: %w", err)
	}
	if j.Workgroup != "jira-report" {
		return errors.New("Jira live disclosure proof must be a jira-report job")
	}
	inputArtifact, err := s.InputArtifact(ctx, j)
	if err != nil {
		return err
	}
	input, err := s.ReadArtifact(inputArtifact, c.Limits.MaxArtifactBytes)
	if err != nil {
		return err
	}
	inputSnapshot, err := jira.ParseReportInput(input)
	if err != nil {
		return err
	}
	if !inputSnapshot.Snapshot.Synthetic {
		return errors.New("Jira live disclosure proof must use a synthetic snapshot")
	}
	required, err := gateway.JiraSourceIDs(input)
	if err != nil {
		return err
	}
	if len(required) == 0 {
		return errors.New("Jira live disclosure proof requires at least one synthetic Jira source")
	}
	attempts, err := s.Attempts(ctx, j.ID)
	if err != nil {
		return err
	}
	artifacts, err := s.Artifacts(ctx, j.ID)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		if attempt.Status != store.Completed {
			continue
		}
		if err := verifyJiraSyntheticAttempt(s, c, j, attempt, artifacts, required, requiredDigest, requiredSkill); err == nil {
			return nil
		}
	}
	return errors.New("Jira live disclosure proof has no completed synthetic attempt with the selected Skill, matching executor, allowed tool result, and every source lookup")
}

func verifyJiraSyntheticAttempt(s *store.Store, c config.Config, j store.Job, attempt store.Attempt, artifacts []store.Artifact, required []string, requiredDigest string, requiredSkill workgroup.Skill) error {
	var manifest workgroup.Package
	var result executor.Result
	lookedUp := map[string]bool{}
	for _, artifact := range artifacts {
		if artifact.AttemptID != attempt.ID {
			continue
		}
		data, err := s.ReadArtifact(artifact, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		switch artifact.Kind {
		case "package_manifest":
			if err = mail.Decode(data, &manifest); err != nil {
				return err
			}
		case "executor_result":
			if err = mail.Decode(data, &result); err != nil {
				return err
			}
		case "source_lookup":
			var evidence gateway.Evidence
			if err = mail.Decode(data, &evidence); err != nil {
				return err
			}
			if evidence.Tool != gateway.JiraToolName || evidence.Result.Issue == nil {
				return errors.New("synthetic Jira proof has invalid lookup evidence")
			}
			if evidence.Result.Issue.ID != evidence.SourceID {
				return errors.New("synthetic Jira proof lookup does not bind its source ID")
			}
			lookedUp[evidence.SourceID] = true
		}
	}
	if manifest.Version != workgroup.BundleVersion || manifest.JobID != j.ID || manifest.AttemptID != attempt.ID || manifest.Workgroup != "jira-report" || !manifest.Synthetic {
		return errors.New("synthetic Jira proof package is invalid")
	}
	bundle, err := workgroup.LoadFor(s.Root, "jira-report", manifest.WorkgroupDigest, c.Limits.MaxArtifactBytes)
	if err != nil {
		return fmt.Errorf("load synthetic Jira proof bundle: %w", err)
	}
	skill, err := bundle.SelectedSkill()
	if err != nil {
		return err
	}
	if manifest.Skill.Name != skill.Name || manifest.Skill.Description != skill.Description || manifest.Skill.Digest != files.Digest([]byte(skill.Markdown)) {
		return errors.New("synthetic Jira proof package does not contain its selected Jira Skill")
	}
	if requiredDigest != "" && (manifest.WorkgroupDigest != requiredDigest || manifest.Skill.Name != requiredSkill.Name || manifest.Skill.Description != requiredSkill.Description || manifest.Skill.Digest != files.Digest([]byte(requiredSkill.Markdown))) {
		return errors.New("synthetic Jira proof does not match the selected live Jira Skill")
	}
	if manifest.Executor.Kind != c.Executor.Kind || manifest.Executor.Path != c.Executor.Path || manifest.Executor.Model != c.Executor.Model {
		return errors.New("synthetic Jira proof used a different executor configuration")
	}
	if result.Outcome != "generated" || result.ExitCode != 0 || len(result.ObservedTools) == 0 {
		return errors.New("synthetic Jira proof has no successful observed tool result")
	}
	for _, tool := range result.ObservedTools {
		if tool != executor.PermittedJiraTool {
			return errors.New("synthetic Jira proof observed a forbidden tool")
		}
	}
	for _, id := range required {
		if !lookedUp[id] {
			return errors.New("synthetic Jira proof did not preserve every required source lookup")
		}
	}
	return nil
}

func missingRequiredSourceLookups(snapshot mail.Snapshot, observed map[string]bool) bool {
	for _, source := range snapshot.Messages {
		if source.Scope == mail.Target && !observed[source.ID] {
			return true
		}
	}
	return false
}

func (r *Runner) collectLookups(ctx context.Context, j store.Job, a store.Attempt) (map[string]bool, error) {
	observed := map[string]bool{}
	preserved := map[string]bool{}
	artifacts, err := r.Store.Artifacts(ctx, j.ID)
	if err != nil {
		return nil, err
	}
	for _, art := range artifacts {
		if art.AttemptID == a.ID && art.Kind == "source_lookup" {
			preserved[art.Digest] = true
		}
	}
	relative := gateway.EvidenceDir(j.ID, a.ID)
	entries, err := os.ReadDir(filepath.Join(r.Store.Root, relative))
	if errors.Is(err, os.ErrNotExist) {
		return observed, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > r.Config.Limits.MaxToolCalls {
		return nil, errors.New("lookup evidence count exceeds budget")
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("invalid lookup evidence entry")
		}
		b, e := files.Read(r.Store.Root, filepath.Join(relative, entry.Name()), r.Config.Limits.MaxArtifactBytes)
		if e != nil {
			return nil, e
		}
		total += int64(len(b))
		if total > r.Config.Limits.MaxEvidenceBytes {
			return nil, errors.New("lookup evidence exceeds total byte budget")
		}
		var evidence gateway.Evidence
		if e = mail.Decode(b, &evidence); e != nil {
			return nil, e
		}
		expectedTool, expectedErr := gateway.ToolForWorkgroup(j.Workgroup)
		if expectedErr != nil || evidence.Tool != expectedTool {
			return nil, errors.New("unknown lookup evidence tool")
		}
		if evidence.Result.Source != nil && evidence.Result.Source.ID == evidence.SourceID {
			observed[evidence.SourceID] = true
		}
		if evidence.Result.Issue != nil {
			if evidence.Result.Issue.ID != evidence.SourceID {
				return nil, errors.New("jira_lookup_evidence_decode")
			}
			observed[evidence.SourceID] = true
		}
		if preserved[files.Digest(b)] {
			continue
		}
		if _, e = r.Store.SaveArtifact(ctx, j.ID, a.ID, "source_lookup", b, r.Config.Limits.MaxArtifactBytes); e != nil {
			return nil, e
		}
	}
	return observed, nil
}

func (r *Runner) completeJiraResult(ctx context.Context, out Outcome, finish func(string, string, bool) error, j store.Job, a store.Attempt, p workgroup.Package, raw []byte, observed map[string]bool) (Outcome, error) {
	if p.JiraSnapshot == nil {
		return out, errors.New("Jira package is missing its pinned input")
	}
	report, validation, validationErr := jira.ValidateReport(raw, p.Bundle.Schema, *p.JiraSnapshot, observed)
	if validationErr != nil {
		validation.Gaps = append(validation.Gaps, validationErr.Error())
	}
	b, _ := json.MarshalIndent(validation, "", "  ")
	if _, err := r.Store.SaveArtifact(ctx, j.ID, a.ID, "validation", b, r.Config.Limits.MaxArtifactBytes); err != nil {
		return out, err
	}
	inputArt, err := r.Store.InputArtifact(ctx, j)
	if err != nil {
		return out, err
	}
	input, err := r.Store.ReadArtifact(inputArt, r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return out, err
	}
	ids, err := gateway.JiraSourceIDs(input)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		if !observed[id] {
			cause := errors.New(requiredSourceLookupsMissing)
			e := finish(store.Failed, requiredSourceLookupsMissing, false)
			return out, errors.Join(cause, e)
		}
	}
	if validationErr != nil {
		e := finish(store.Failed, "result contract: "+validationErr.Error(), true)
		return out, errors.Join(validationErr, e)
	}
	index, err := jira.BuildSourceIndex(*p.JiraSnapshot)
	if err != nil {
		return out, err
	}
	sourceData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return out, err
	}
	sources, err := r.Store.SaveArtifact(ctx, j.ID, a.ID, "report_sources", sourceData, r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		e := finish(store.WaitingInput, store.PublicationRequired, false)
		return out, errors.Join(err, e)
	}
	art, err := r.Store.SaveArtifact(ctx, j.ID, a.ID, "report_markdown", jira.RenderMarkdown(report, validation, index, p.JiraSnapshot.Policy.TodoStatusID, filepath.Base(sources.Path)), r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		e := finish(store.WaitingInput, store.PublicationRequired, false)
		return out, errors.Join(err, e)
	}
	out.ReportPath = filepath.Join(r.Store.Root, art.Path)
	if err = r.Store.AddEvent(ctx, j.ID, a.ID, "report.available", map[string]string{"artifact_id": art.ID, "availability": "local", "acknowledgment": "unknown"}); err != nil {
		return out, err
	}
	if err = finish(validation.OperationalStatus, "", false); err != nil {
		return out, err
	}
	return out, nil
}

func Recover(ctx context.Context, s *store.Store, c config.Config) (int, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if j.CurrentAttempt == "" {
			continue
		}
		path := filepath.Join("runs", j.ID, "attempts", j.CurrentAttempt, executor.ProcessFile)
		b, e := files.Read(s.Root, path, c.Limits.MaxArtifactBytes)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return 0, e
		}
		var record executor.ProcessRecord
		if e = mail.Decode(b, &record); e != nil {
			return 0, e
		}
		if record.State == "starting" {
			return 0, errors.New("executor start was interrupted before identity was recorded; inspect surviving processes before resuming")
		}
		if record.State == "not_started" || record.State == "exited" {
			continue
		}
		if record.State != "running" {
			return 0, errors.New("unknown executor process state")
		}
		cleanupCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		e = platform.ReconcileProcess(cleanupCtx, record.Identity)
		cancel()
		if e != nil {
			return 0, fmt.Errorf("job %s: %w", j.ID, e)
		}
		record.State = "exited"
		b, e = json.Marshal(record)
		if e != nil {
			return 0, e
		}
		if e = files.Write(s.Root, path, b, true); e != nil {
			return 0, e
		}
	}
	return s.RecoverInterrupted(ctx)
}
