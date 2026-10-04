package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/conversationstate"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"chunsu/internal/stagedworkflow"
	"chunsu/internal/store"
	"chunsu/internal/webresearch"
	"chunsu/internal/workgroup"
)

func stageOrigin(req control.Request) (stagedworkflow.Origin, error) {
	if !files.ValidID(req.ConversationID) || !files.ValidID(req.MessageID) || req.RequestRevision <= 0 {
		return stagedworkflow.Origin{}, errors.New("staged job requires a durable originating conversation and message")
	}
	return stagedworkflow.Origin{ConversationID: req.ConversationID, MessageID: req.MessageID, RequestRevision: req.RequestRevision, Destination: "origin_conversation", ManualCheckpoint: req.ManualCheckpoint}, nil
}

func (r *Runner) submitSavedStage(ctx context.Context, group string, input []byte, request any, req control.Request) (store.Job, error) {
	c, err := config.Load(r.Store.Root)
	if err != nil {
		return store.Job{}, err
	}
	if c.ModelPolicyVersion != 1 {
		return store.Job{}, errors.New("staged work needs an explicit conversation model policy; use config policy apply after reviewing routes")
	}
	origin, err := stageOrigin(req)
	if err != nil {
		return store.Job{}, err
	}
	scope, digest, err := r.resolveSkills(group, req)
	if err != nil {
		return store.Job{}, err
	}
	var selections []stagedworkflow.SkillSelection
	if scope != nil {
		selections = append(selections, stagedworkflow.SkillSelection{Scope: scope, Digest: digest})
	}
	spec, err := stagedworkflow.BuildSpec(r.Store.Root, group, req.Answer, input, c, origin, selections...)
	if err != nil {
		return store.Job{}, err
	}
	pinnedRequest, err := scopedRequest(request, scope, digest)
	if err != nil {
		return store.Job{}, err
	}
	return r.Store.SubmitStaged(ctx, group, input, pinnedRequest, spec, c.Limits.MaxArtifactBytes)
}

func (r *Runner) submitWebStage(ctx context.Context, req control.Request) (store.Job, error) {
	if err := webresearch.ValidateQuery(req.Answer); err != nil {
		return store.Job{}, err
	}
	c, err := config.Load(r.Store.Root)
	if err != nil {
		return store.Job{}, err
	}
	if c.ModelPolicyVersion != 1 {
		return store.Job{}, errors.New("staged work needs an explicit conversation model policy; use config policy apply after reviewing routes")
	}
	origin, err := stageOrigin(req)
	if err != nil {
		return store.Job{}, err
	}
	input := []byte("{}")
	spec, err := stagedworkflow.BuildSpec(r.Store.Root, stagedworkflow.WebWorkgroup, req.Answer, input, c, origin)
	if err != nil {
		return store.Job{}, err
	}
	return r.Store.SubmitStaged(ctx, stagedworkflow.WebWorkgroup, input, map[string]any{"origin": "public_web", "admission": "reception"}, spec, c.Limits.MaxArtifactBytes)
}

func (r *Runner) readResultEvent(ctx context.Context, eventID, conversationID string) (store.ResultEvent, error) {
	e, err := r.authorizedResultEvent(ctx, eventID, conversationID)
	if err != nil {
		return e, err
	}
	job, err := r.Store.Job(ctx, e.JobID)
	if err != nil {
		return e, err
	}
	artifacts, err := r.Store.Artifacts(ctx, e.JobID)
	if err != nil {
		return e, err
	}
	var found bool
	var validated stagedworkflow.Validated
	for _, artifact := range artifacts {
		if artifact.ID != e.ArtifactID {
			continue
		}
		if artifact.Digest != e.ArtifactDigest {
			return e, errors.New("result event artifact digest changed")
		}
		data, readErr := r.Store.ReadArtifact(artifact, r.Config.Limits.MaxArtifactBytes)
		if readErr != nil {
			return e, readErr
		}
		if err = json.Unmarshal(data, &validated); err != nil {
			return e, err
		}
		if validated.Version != 1 || validated.Workgroup != job.Workgroup || validated.Markdown == "" {
			return e, errors.New("validated result projection is incomplete")
		}
		found = true
		break
	}
	if !found {
		return e, errors.New("result event artifact is unavailable")
	}
	c, err := config.Load(r.Store.Root)
	if err != nil {
		return e, err
	}
	if !receptionMayRead(job.Workgroup, c) {
		e.Summary = "A validated result is available for this job. Private details require a separately approved reception route."
		e.SourceReferences = json.RawMessage(`[]`)
		e.Gaps = json.RawMessage(`[]`)
		e.RequiredDecision = nil
	} else {
		e.Summary = boundedResult(validated.Markdown, store.MaxResultSummaryBytes)
	}
	return e, nil
}

func (r *Runner) authorizedResultEvent(ctx context.Context, eventID, conversationID string) (store.ResultEvent, error) {
	candidate, err := r.Store.ResultEvent(ctx, eventID)
	if err != nil {
		return candidate, err
	}
	e, err := r.Store.ResultEventForHandoff(ctx, eventID, candidate.ConversationID)
	if err != nil || e.ConversationID == conversationID {
		return e, err
	}
	state := conversationstate.New(r.Store.Root, r.Config.Limits)
	current, err := state.Load(conversationID)
	if err != nil {
		return e, err
	}
	prior, err := state.Load(e.ConversationID)
	if err != nil {
		return e, err
	}
	if current.Archived || current.Channel != prior.Channel || current.Owner != prior.Owner || !slices.Contains(current.JobIDs, e.JobID) || !slices.Contains(prior.JobIDs, e.JobID) {
		return e, errors.New("result event is not selected for this conversation")
	}
	return e, nil
}

func (r *Runner) resultEventsForConversation(ctx context.Context, conversationID string, sending bool) ([]store.ResultEvent, error) {
	state := conversationstate.New(r.Store.Root, r.Config.Limits)
	current, err := state.Load(conversationID)
	if err != nil {
		return nil, err
	}
	if current.Archived {
		return nil, errors.New("archived conversations do not receive automatic results")
	}
	list := r.Store.PendingResultEvents
	if sending {
		list = r.Store.SendingResultEvents
	}
	events, err := list(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.ID] = true
	}
	for _, jobID := range current.JobIDs {
		workflow, workflowErr := r.Store.Workflow(ctx, jobID)
		if errors.Is(workflowErr, sql.ErrNoRows) {
			continue
		}
		if workflowErr != nil {
			return nil, workflowErr
		}
		if workflow.OriginConversationID == conversationID {
			continue
		}
		prior, loadErr := state.Load(workflow.OriginConversationID)
		if loadErr != nil {
			return nil, loadErr
		}
		if prior.Channel != current.Channel || prior.Owner != current.Owner || !slices.Contains(prior.JobIDs, jobID) {
			return nil, errors.New("selected job origin no longer matches this conversation")
		}
		priorEvents, listErr := list(ctx, prior.ID)
		if listErr != nil {
			return nil, listErr
		}
		for _, event := range priorEvents {
			if event.JobID == jobID && !seen[event.ID] {
				events = append(events, event)
				seen[event.ID] = true
			}
		}
	}
	return events, nil
}

func boundedResult(markdown string, limit int) string {
	const suffix = "\n\n[Result shortened here. Use `flow result EVENT_ID` on the owner CLI for the complete validated report.]"
	if len(markdown) <= limit {
		return markdown
	}
	limit -= len(suffix)
	if limit <= 0 {
		return suffix
	}
	for !utf8.ValidString(markdown[:limit]) {
		limit--
	}
	return strings.TrimSpace(markdown[:limit]) + suffix
}

func receptionMayRead(group string, c config.Config) bool {
	if group == stagedworkflow.WebWorkgroup {
		return true
	}
	route := c.ExecutorFor(config.RoleReception)
	switch group {
	case "mail-review":
		return route.LiveMailApproved
	case "jira-report":
		return false // Jira details use host-owned result inspection; no reception proof route exists.
	case "code-review":
		return route.LiveCodeApproved
	}
	return false
}

// RunStage executes exactly one eligible step. The same active slot and cancel
// function used by legacy jobs keep the controller at one child process.
func (r *Runner) RunStage(ctx context.Context, jobID string) (out Outcome, runErr error) {
	out.JobID = jobID
	r.admission.Lock()
	r.mu.Lock()
	if r.active != "" || r.controller && (!r.dispatch || r.stopping) {
		r.mu.Unlock()
		r.admission.Unlock()
		return out, errors.New("worker is not available for staged work")
	}
	paused, err := r.Store.Paused(ctx)
	if err != nil || paused {
		r.mu.Unlock()
		r.admission.Unlock()
		if err != nil {
			return out, err
		}
		out.Status = store.Staged
		return out, nil
	}
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(r.Config.Limits.TimeoutSeconds)*time.Second)
	r.active, r.cancel = jobID, cancel
	r.mu.Unlock()
	r.admission.Unlock()
	defer func() { cancel(); r.mu.Lock(); r.active = ""; r.cancel = nil; r.mu.Unlock() }()
	finishCtx := context.WithoutCancel(ctx)
	step, err := r.Store.ReadyStep(attemptCtx, jobID)
	if errors.Is(err, sql.ErrNoRows) {
		out.Status = store.Staged
		return out, r.publishValidatedResult(finishCtx, jobID)
	}
	if err != nil {
		return out, err
	}
	attempt, err := r.Store.ClaimStep(attemptCtx, step.ID, step.CurrentAttemptID)
	if err != nil {
		return out, err
	}
	out.AttemptID, out.Status = attempt.ID, store.StepRunning
	current, err := config.Load(r.Store.Root)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	job, err := r.Store.Job(attemptCtx, jobID)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	workflow, err := r.Store.Workflow(attemptCtx, jobID)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	var objective string
	if err = json.Unmarshal(workflow.OriginalRequest, &objective); err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	var scope struct {
		BundleDigest string                `json:"bundle_digest"`
		MailMode     string                `json:"mail_mode"`
		SkillScope   *workgroup.SkillScope `json:"skill_scope"`
	}
	if err = json.Unmarshal(workflow.SourceScope, &scope); err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	inputArtifact, err := r.Store.InputArtifact(attemptCtx, job)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	snapshot, err := r.Store.ReadArtifact(inputArtifact, current.Limits.MaxArtifactBytes)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	if job.Workgroup == "jira-report" && (step.Stage == stagedworkflow.Refine || step.Stage == stagedworkflow.Synthesize) {
		report, parseErr := jira.ParseReportInput(snapshot)
		if parseErr != nil {
			return r.failStage(finishCtx, out, step, attempt, parseErr)
		}
		if !report.Snapshot.Synthetic {
			role := config.RoleRefinement
			if step.Stage == stagedworkflow.Synthesize {
				role = config.RoleSynthesis
			}
			route := current.ExecutorFor(role)
			if !route.LiveJiraApproved || route.LiveJiraPolicyDigest != report.ReportPolicyDigest {
				return r.failStage(finishCtx, out, step, attempt, errors.New("live Jira stage route or policy is not approved"))
			}
			if proofErr := VerifyStagedJiraProof(attemptCtx, r.Store, current, route.LiveJiraValidationJobID, role, report.ReportPolicyDigest, stagedworkflow.SkillSelection{Scope: scope.SkillScope, Digest: scope.BundleDigest}); proofErr != nil {
				return r.failStage(finishCtx, out, step, attempt, proofErr)
			}
		}
	}
	inputs, err := r.stageInputs(attemptCtx, jobID, attempt)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	in := stagedworkflow.StageInput{Root: r.Store.Root, JobID: jobID, StepID: step.ID, AttemptID: attempt.ID, Workgroup: job.Workgroup, Stage: step.Stage, Objective: objective, Request: job.Request, Snapshot: snapshot, Config: current, Inputs: inputs, PinnedModel: attempt.ExecutorModel, PinnedEffort: attempt.ExecutorEffort, PinnedIdentity: attempt.ExecutorIdentity, PinnedBundleDigest: scope.BundleDigest, PinnedMailMode: scope.MailMode}
	in.PinnedSkillScope = scope.SkillScope
	result, stageErr := stagedworkflow.ExecuteStage(attemptCtx, in)
	var receiptID string
	if result.Receipt != nil {
		b, marshalErr := json.Marshal(result.Receipt)
		if marshalErr != nil {
			stageErr = errors.Join(stageErr, marshalErr)
		} else {
			receipt, saveErr := r.Store.SaveStepArtifact(finishCtx, step.ID, attempt.ID, "staged_executor_receipt", b, current.Limits.MaxArtifactBytes)
			if saveErr != nil {
				stageErr = errors.Join(stageErr, saveErr)
			} else {
				receiptID = receipt.ID
			}
		}
	}
	if stageErr != nil {
		return r.failStage(finishCtx, out, step, attempt, stageErr)
	}
	if result.Status != store.StepCompleted || result.Kind == "" || len(result.Data) == 0 {
		return r.failStage(finishCtx, out, step, attempt, errors.New("stage did not produce a validated artifact"))
	}
	for kind, data := range result.Extra {
		if _, err = r.Store.SaveStepArtifact(finishCtx, step.ID, attempt.ID, kind, data, current.Limits.MaxArtifactBytes); err != nil {
			return r.failStage(finishCtx, out, step, attempt, err)
		}
	}
	artifact, err := r.Store.SaveStepArtifact(finishCtx, step.ID, attempt.ID, result.Kind, result.Data, current.Limits.MaxArtifactBytes)
	if err != nil {
		return r.failStage(finishCtx, out, step, attempt, err)
	}
	if _, err = r.Store.CompleteStep(finishCtx, step.ID, attempt.ID, artifact.ID, store.StepCompleted); err != nil {
		if errors.Is(err, store.ErrStaleStep) {
			out.Status = store.StepCancelled
			return out, nil
		}
		return out, err
	}
	_ = receiptID
	out.Status = store.StepCompleted
	if step.Stage == stagedworkflow.Deliver {
		return out, r.publishValidatedResult(finishCtx, jobID)
	}
	return out, nil
}

func (r *Runner) failStage(ctx context.Context, out Outcome, step store.Step, attempt store.StepAttempt, cause error) (Outcome, error) {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, store.ErrStaleStep) {
		out.Status = store.StepCancelled
		return out, nil
	}
	_, err := r.Store.CompleteStep(ctx, step.ID, attempt.ID, "", store.StepFailed)
	if errors.Is(err, store.ErrStaleStep) {
		out.Status = store.StepCancelled
		return out, nil
	}
	out.Status = store.StepFailed
	out.Diagnostic = "staged step failed; inspect the step and attempt receipts before retrying"
	return out, errors.Join(cause, err)
}

func (r *Runner) stageInputs(ctx context.Context, jobID string, attempt store.StepAttempt) (map[string]stagedworkflow.PinnedArtifact, error) {
	steps, err := r.Store.Steps(ctx, jobID)
	if err != nil {
		return nil, err
	}
	artifacts, err := r.Store.Artifacts(ctx, jobID)
	if err != nil {
		return nil, err
	}
	byID := map[string]store.Artifact{}
	for _, artifact := range artifacts {
		byID[artifact.ID] = artifact
	}
	inputs := map[string]stagedworkflow.PinnedArtifact{}
	for _, ref := range attempt.Inputs {
		artifact, ok := byID[ref.ArtifactID]
		if !ok || artifact.Digest != ref.Digest {
			return nil, errors.New("pinned staged input changed")
		}
		data, err := r.Store.ReadArtifact(artifact, r.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return nil, err
		}
		key := "input"
		for _, step := range steps {
			if step.OutputArtifactID == artifact.ID {
				key = step.Key
				break
			}
		}
		inputs[key] = stagedworkflow.PinnedArtifact{ID: artifact.ID, Kind: artifact.Kind, Digest: artifact.Digest, Data: data}
	}
	return inputs, nil
}

func (r *Runner) publishValidatedResult(ctx context.Context, jobID string) error {
	steps, err := r.Store.Steps(ctx, jobID)
	if err != nil {
		return err
	}
	var validated store.Step
	var delivered bool
	for _, step := range steps {
		if step.State != store.StepCompleted {
			return nil
		}
		if step.Key == stagedworkflow.Validate {
			validated = step
		}
		if step.Key == stagedworkflow.Deliver {
			delivered = true
		}
	}
	if !delivered || validated.OutputArtifactID == "" {
		return nil
	}
	artifacts, err := r.Store.Artifacts(ctx, jobID)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if artifact.ID != validated.OutputArtifactID {
			continue
		}
		data, readErr := r.Store.ReadArtifact(artifact, r.Config.Limits.MaxArtifactBytes)
		if readErr != nil {
			return readErr
		}
		var report stagedworkflow.Validated
		if err = json.Unmarshal(data, &report); err != nil {
			return err
		}
		if report.Version != 1 || report.Workgroup == "" || report.Markdown == "" {
			return errors.New("validated result envelope is incomplete")
		}
		refs, _ := json.Marshal(report.SourceIDs)
		gaps, _ := json.Marshal(report.Gaps)
		_, err = r.Store.PublishResultEvent(ctx, store.ResultEventInput{JobID: jobID, StepID: validated.ID, ArtifactID: artifact.ID, Summary: fmt.Sprintf("Validated %s result is available", report.Workgroup), SourceReferences: refs, Gaps: gaps, Status: report.Status})
		return err
	}
	return errors.New("validated result artifact is missing")
}

func RecoverStages(ctx context.Context, s *store.Store, c config.Config) (int, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		if _, err = s.Workflow(ctx, job.ID); errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		steps, err := s.Steps(ctx, job.ID)
		if err != nil {
			return 0, err
		}
		for _, step := range steps {
			attempts, attemptErr := s.StepAttempts(ctx, step.ID)
			if attemptErr != nil {
				return 0, attemptErr
			}
			for _, attempt := range attempts {
				// A cancel can reach SQLite before it reaches the child. Scan
				// terminal attempts too: Reconcile checks the recorded process
				// identity and never signals a process by PID alone.
				for _, role := range []string{config.RoleCollection, config.RoleRefinement, config.RoleSynthesis} {
					path := filepath.Join("runs", job.ID, "steps", step.ID, "attempts", attempt.ID, role, executor.ProcessFile)
					if err = executor.Reconcile(ctx, s.Root, path, c.Limits); err != nil {
						return 0, err
					}
				}
			}
		}
	}
	return s.RecoverInterruptedSteps(ctx)
}
