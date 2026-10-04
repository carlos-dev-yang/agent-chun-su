package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"chunsu/internal/stagedworkflow"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

// VerifyStagedJiraProof is the staged counterpart of the legacy Jira proof:
// the owner-selected route must have completed an actual synthetic Jira model
// step against the still-selected Skill and policy, with a validated result.
func VerifyStagedJiraProof(ctx context.Context, s *store.Store, c config.Config, jobID, role, policyDigest string, required ...stagedworkflow.SkillSelection) error {
	if !files.ValidID(jobID) || !files.ValidDigest(policyDigest) {
		return errors.New("a synthetic Jira proof job and policy digest are required")
	}
	if role != config.RoleRefinement && role != config.RoleSynthesis {
		return errors.New("this role has no staged Jira proof step")
	}
	job, err := s.Job(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Workgroup != "jira-report" {
		return errors.New("Jira proof must be a jira-report staged job")
	}
	w, err := s.Workflow(ctx, jobID)
	if err != nil {
		return err
	}
	if w.WorkflowVersion != stagedworkflow.Version {
		return errors.New("Jira proof workflow version differs")
	}
	inputArtifact, err := s.InputArtifact(ctx, job)
	if err != nil {
		return err
	}
	input, err := s.ReadArtifact(inputArtifact, c.Limits.MaxArtifactBytes)
	if err != nil {
		return err
	}
	report, err := jira.ParseReportInput(input)
	if err != nil {
		return err
	}
	if !report.Snapshot.Synthetic || report.ReportPolicyDigest != policyDigest {
		return errors.New("Jira proof snapshot is not synthetic or uses another policy")
	}
	var scope struct {
		BundleDigest string                `json:"bundle_digest"`
		SkillScope   *workgroup.SkillScope `json:"skill_scope"`
	}
	if err = json.Unmarshal(w.SourceScope, &scope); err != nil {
		return err
	}
	if len(required) > 1 {
		return errors.New("Jira proof requires one exact Skill selection")
	}
	activeDigest := ""
	if len(required) == 0 {
		_, activeDigest, err = workgroup.ActiveInScope(s.Root, job.Workgroup, scope.SkillScope, c.Limits.MaxArtifactBytes)
	} else {
		activeDigest = required[0].Digest
		if (required[0].Scope == nil) != (scope.SkillScope == nil) || (scope.SkillScope != nil && *scope.SkillScope != *required[0].Scope) {
			return errors.New("Jira proof belongs to a different Skill application scope")
		}
		bundle, loadErr := workgroup.LoadFor(s.Root, job.Workgroup, activeDigest, c.Limits.MaxArtifactBytes)
		err = loadErr
		if err == nil && scope.SkillScope != nil {
			err = workgroup.ValidateScopeBundle(bundle, *scope.SkillScope)
		}
	}
	if err != nil {
		return err
	}
	if scope.BundleDigest != activeDigest {
		return errors.New("selected Jira Skill changed after synthetic proof")
	}
	steps, err := s.Steps(ctx, jobID)
	if err != nil {
		return err
	}
	wantStage := stagedworkflow.Refine
	if role == config.RoleSynthesis {
		wantStage = stagedworkflow.Synthesize
	}
	var proven, validated store.Step
	for _, step := range steps {
		if step.Stage == wantStage {
			proven = step
		}
		if step.Stage == stagedworkflow.Validate {
			validated = step
		}
	}
	if proven.ID == "" || proven.State != store.StepCompleted || validated.ID == "" || validated.State != store.StepCompleted {
		return errors.New("synthetic Jira proof has incomplete model or validation steps")
	}
	if err = stagedworkflow.ValidatePinnedRoute(role, proven.ExecutorModel, proven.ExecutorEffort, proven.ExecutorIdentity, c); err != nil {
		return err
	}
	attempts, err := s.StepAttempts(ctx, proven.ID)
	if err != nil {
		return err
	}
	var completed store.StepAttempt
	for _, attempt := range attempts {
		if attempt.ID == proven.CurrentAttemptID && attempt.Status == store.StepCompleted {
			completed = attempt
			break
		}
	}
	if completed.ID == "" {
		return errors.New("Jira proof has no completed current model attempt")
	}
	artifacts, err := s.Artifacts(ctx, jobID)
	if err != nil {
		return err
	}
	var receipt executor.Result
	var foundReceipt, foundValidated bool
	for _, artifact := range artifacts {
		if artifact.ID == validated.OutputArtifactID {
			data, e := s.ReadArtifact(artifact, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
			}
			var result stagedworkflow.Validated
			if e = json.Unmarshal(data, &result); e != nil {
				return e
			}
			if result.Version != 1 || result.Workgroup != "jira-report" || result.Markdown == "" || len(result.SourceIDs) == 0 {
				return errors.New("Jira proof validated result is incomplete")
			}
			foundValidated = true
		}
		if artifact.AttemptID == completed.ID && artifact.Kind == "staged_executor_receipt" {
			data, e := s.ReadArtifact(artifact, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
			}
			if e = json.Unmarshal(data, &receipt); e != nil {
				return e
			}
			foundReceipt = true
		}
	}
	if !foundReceipt || !foundValidated || receipt.Role != role || receipt.Outcome != "generated" || receipt.ExitCode != 0 || receipt.ArgumentsDigest == "" || len(receipt.ObservedTools) != 0 {
		return fmt.Errorf("Jira proof lacks a successful tool-free %s receipt and validated result", role)
	}
	return nil
}
