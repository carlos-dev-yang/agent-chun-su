package runner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

func (r *Runner) saveModulePresentation(ctx context.Context, jobID, attemptID string, plan workgroup.ReportPlan) (store.Artifact, error) {
	if _, err := r.Store.SaveArtifact(ctx, jobID, attemptID, "report_sources", plan.Sources, r.Config.Limits.MaxArtifactBytes); err != nil {
		return store.Artifact{}, err
	}
	return r.Store.SaveArtifact(ctx, jobID, attemptID, "report_markdown", plan.Markdown, r.Config.Limits.MaxArtifactBytes)
}

func (r *Runner) completeModuleResult(ctx context.Context, out Outcome, finish func(string, string, bool) error, p workgroup.Package, raw []byte, observed map[string]bool, definition workgroup.Definition) (Outcome, error) {
	plan, validationErr := definition.ValidateResult(raw, p.Bundle.Schema, p.Input, p.Limits, observed)
	data, err := json.MarshalIndent(plan.Validation, "", "  ")
	if err != nil {
		return out, err
	}
	if _, err = r.Store.SaveArtifact(ctx, p.JobID, p.AttemptID, "validation", data, r.Config.Limits.MaxArtifactBytes); err != nil {
		return out, err
	}
	if validationErr != nil {
		return out, errors.Join(validationErr, finish(store.Failed, "result contract: "+validationErr.Error(), false))
	}
	artifact, err := r.saveModulePresentation(ctx, p.JobID, p.AttemptID, plan)
	if err != nil {
		return out, errors.Join(err, finish(store.WaitingInput, store.PublicationRequired, false))
	}
	out.ReportPath = filepath.Join(r.Store.Root, artifact.Path)
	if err = r.Store.AddEvent(ctx, p.JobID, p.AttemptID, "report.available", map[string]string{"artifact_id": artifact.ID, "availability": "local", "acknowledgment": "unknown"}); err != nil {
		return out, err
	}
	return out, finish(plan.Status, "", false)
}
