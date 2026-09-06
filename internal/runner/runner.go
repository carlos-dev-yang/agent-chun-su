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
		if _, err := mail.ParseSnapshot(req.Input, r.Config.Limits); err != nil {
			return nil, err
		}
		return r.Store.Submit(ctx, mail.Workgroup, req.Input, map[string]any{"origin": "saved", "admission": "management"}, r.Config.Limits.MaxArtifactBytes)
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
		return map[string]any{"active_job": id, "owner": "controller"}, nil
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
	report, validation, validationErr := mail.ValidateReport(generated.Final, p.Bundle.Schema, p.Snapshot, p.Mode, observed)
	if validationErr != nil {
		validation.Gaps = append(validation.Gaps, validationErr.Error())
	}
	b, _ := json.MarshalIndent(validation, "", "  ")
	if _, err = r.Store.SaveArtifact(finishCtx, jobID, a.ID, "validation", b, r.Config.Limits.MaxArtifactBytes); err != nil {
		return out, err
	}
	if validationErr != nil {
		e := finish(store.Failed, "result contract: "+validationErr.Error(), true)
		return out, errors.Join(validationErr, e)
	}
	art, err := r.Store.SaveArtifact(finishCtx, jobID, a.ID, "report_markdown", mail.Render(report, validation, p.Snapshot.Synthetic), r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		e := finish(store.WaitingInput, "local report publication failed; use publish to retry presentation", false)
		return out, errors.Join(err, e)
	}
	out.ReportPath = filepath.Join(r.Store.Root, art.Path)
	if err = r.Store.AddEvent(finishCtx, jobID, a.ID, "report.available", map[string]string{"artifact_id": art.ID, "availability": "local", "acknowledgment": "unknown"}); err != nil {
		return out, err
	}
	return out, finish(validation.OperationalStatus, "", false)
}

func (r *Runner) collectLookups(ctx context.Context, j store.Job, a store.Attempt) (map[string]bool, error) {
	observed := map[string]bool{}
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
		if evidence.Tool != gateway.ToolName {
			return nil, errors.New("unknown lookup evidence tool")
		}
		if evidence.Result.Source != nil && evidence.Result.Source.ID == evidence.SourceID {
			observed[evidence.SourceID] = true
		}
		if _, e = r.Store.SaveArtifact(ctx, j.ID, a.ID, "source_lookup", b, r.Config.Limits.MaxArtifactBytes); e != nil {
			return nil, e
		}
	}
	return observed, nil
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
