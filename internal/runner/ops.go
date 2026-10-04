package runner

import (
	"context"
	"encoding/json"
	"errors"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/feedback"
	"chunsu/internal/mail"
	"chunsu/internal/ops"
	"chunsu/internal/stagedworkflow"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

func (r *Runner) handleOps(ctx context.Context, req control.Request) (any, error) {
	c, err := config.Load(r.Store.Root)
	if err != nil {
		return nil, err
	}
	service := ops.Service{Store: r.Store, Config: c}
	switch req.Operation {
	case "ops_command":
		var command ops.Command
		if err = mail.Decode(req.Input, &command); err != nil {
			return nil, err
		}
		return service.Execute(ctx, command)
	case "ops_report":
		var request ops.ReportRequest
		if err = mail.Decode(req.Input, &request); err != nil {
			return nil, err
		}
		input, err := service.BuildInput(ctx, request.Scope, request.AsOf)
		if err != nil {
			return nil, err
		}
		if err = verifyOpsSkillScope(input.Scope, req.SkillScope); err != nil {
			return nil, err
		}
		if err = ops.AuthorizeInput(input, c.ExecutorFor(config.RoleSynthesis)); err != nil {
			return nil, err
		}
		data, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		return r.submitSavedStage(ctx, ops.Workgroup, data, map[string]any{"origin": "saved_team_ops", "admission": "owner_cli", "user_request": req.Answer}, req)
	case "ops_reconcile":
		var command ops.Command
		if err = mail.Decode(req.Input, &command); err != nil {
			return nil, err
		}
		if command.Action != "assessment.apply" || command.EntityID != req.JobID {
			return nil, errors.New("OPS reconciliation requires an exact selected job")
		}
		state, history, err := service.Load(ctx, command.Scope)
		if err != nil {
			return nil, err
		}
		for _, entry := range history {
			if entry.Event.Command.OperationID != command.OperationID {
				continue
			}
			original := entry.Event.Command
			candidate := command
			candidate.Data = original.Data
			left, _ := json.Marshal(candidate)
			right, _ := json.Marshal(original)
			if string(left) != string(right) {
				return nil, errors.New("OPS reconciliation operation ID was used with another command")
			}
			return ops.Receipt{Record: entry.Record, Revision: entry.Event.Revision, OperationID: command.OperationID, Replayed: true}, nil
		}
		if state.Revision != command.ExpectedRevision {
			return nil, errors.New("OPS reconciliation revision changed; inspect the ledger")
		}
		run, err := (feedback.Service{Store: r.Store, Config: c}).InspectRun(ctx, req.JobID)
		if err != nil {
			return nil, err
		}
		if run.Job.Workgroup != ops.Workgroup || run.Flow == nil || run.Flow.RevisionDigest == "" || run.SelectedAttemptID == "" || run.ExecutorResult == nil || run.ExecutorResult.Outcome != "generated" || run.ExecutorResult.ExitCode != 0 {
			return nil, errors.New("OPS reconciliation requires a verified actual AI FLOW report")
		}
		inputBytes, err := r.Store.ReadArtifact(run.Flow.Input, c.Limits.MaxArtifactBytes)
		if err != nil {
			return nil, err
		}
		input, err := ops.ParseInput(inputBytes, c.Limits)
		if err != nil {
			return nil, err
		}
		if input.Scope != command.Scope || input.LedgerRevision != state.Revision || input.LedgerHead != state.Head {
			return nil, errors.New("OPS assessment input is stale or belongs to another scope")
		}
		skillScope, err := json.Marshal(run.Flow.SkillScope)
		if err != nil {
			return nil, err
		}
		if err = verifyOpsSkillScope(input.Scope, skillScope); err != nil {
			return nil, err
		}
		if err = r.verifyOpsInput(ctx, inputBytes, c); err != nil {
			return nil, err
		}
		bundle, err := workgroup.LoadFor(r.Store.Root, ops.Workgroup, run.Flow.BundleDigest, c.Limits.MaxArtifactBytes)
		if err != nil {
			return nil, err
		}
		var artifact store.Artifact
		for _, output := range run.Flow.Outputs {
			if output.Kind == "staged_validated" {
				artifact = output
			}
		}
		if artifact.ID == "" {
			return nil, errors.New("OPS validated report artifact is unavailable")
		}
		data, err := r.Store.ReadArtifact(artifact, c.Limits.MaxArtifactBytes)
		if err != nil {
			return nil, err
		}
		var validated stagedworkflow.Validated
		if err = mail.Decode(data, &validated); err != nil {
			return nil, err
		}
		observed := map[string]bool{}
		for _, id := range validated.SourceIDs {
			observed[id] = true
		}
		report, _, err := ops.ValidateReport(validated.Report, bundle.Schema, input, observed)
		if err != nil {
			return nil, err
		}
		assessment := ops.Assessment{JobID: run.Job.ID, Artifact: ops.RecordRef{ID: artifact.ID, Digest: artifact.Digest}, InputDigest: run.Flow.InputDigest, LedgerRevision: input.LedgerRevision, BundleDigest: run.Flow.BundleDigest, Report: report}
		return service.RecordAssessment(ctx, command, assessment)
	default:
		return nil, errors.New("unsupported OPS controller operation")
	}
}

// Generic flow submit cannot turn a private saved input into a synthetic one by
// changing its serialized flag. The controller rebuilds it from verified raw
// observations and the current scope ledger before admitting an OPS report.
func (r *Runner) verifyOpsInput(ctx context.Context, data []byte, c config.Config) error {
	in, err := ops.ParseInput(data, c.Limits)
	if err != nil {
		return err
	}
	expected, err := (ops.Service{Store: r.Store, Config: c}).BuildInput(ctx, in.Scope, in.AsOf)
	if err != nil {
		return err
	}
	left, _ := json.Marshal(in)
	right, _ := json.Marshal(expected)
	if string(left) != string(right) {
		return errors.New("team-ops admission differs from the host-built ledger and raw-source provenance")
	}
	return ops.AuthorizeInput(expected, c.ExecutorFor(config.RoleSynthesis))
}

// A scope is a Skill-selection namespace, not an authorization grant. An OPS
// report must use the exact project namespace whose ledger it assesses.
func verifyOpsSkillScope(scope ops.Scope, raw json.RawMessage) error {
	var selected workgroup.SkillScope
	if len(raw) == 0 || mail.Decode(raw, &selected) != nil || selected != (workgroup.SkillScope{Kind: "project", Team: scope.Team, Project: scope.Project}) {
		return errors.New("team-ops requires the exact project Skill scope matching its team/project ledger; no missing or cross-project fallback is allowed")
	}
	return selected.Validate()
}

func (r *Runner) verifyOpsAdmission(ctx context.Context, data, rawScope json.RawMessage, c config.Config) error {
	if err := r.verifyOpsInput(ctx, data, c); err != nil {
		return err
	}
	input, err := ops.ParseInput(data, c.Limits)
	if err != nil {
		return err
	}
	return verifyOpsSkillScope(input.Scope, rawScope)
}
