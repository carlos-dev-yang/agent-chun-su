package feedback

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

// ReleasePolicy is a human-owned versioned record, never an executor option.
// It has no implicit pass percentage or fabricated personal quality threshold.
type ReleasePolicy struct {
	Version                     int      `json:"version"`
	Name                        string   `json:"name"`
	Workgroup                   string   `json:"workgroup"`
	ReviewStatus                string   `json:"review_status"`
	Reviewer                    string   `json:"reviewer"`
	RequiredCriteria            []string `json:"required_criteria"`
	AllowedOutcomes             []string `json:"allowed_outcomes"`
	AllowedJobStatuses          []string `json:"allowed_job_statuses"`
	RequireReviewedExpectations bool     `json:"require_reviewed_expectations"`
	RequireConfirmation         bool     `json:"require_confirmation"`
	RequireIsolatedAI           bool     `json:"require_isolated_ai"`
	RejectRegressions           bool     `json:"reject_regressions"`
	AllowEvaluationLimitations  bool     `json:"allow_evaluation_limitations"`
}

func (p ReleasePolicy) Validate() error {
	if p.Version != Version || !mail.Nonempty(p.Name) || !mail.Nonempty(p.Workgroup) || !validReview(p.ReviewStatus, p.Reviewer) {
		return errors.New("release policy needs a version, name, workgroup and review metadata")
	}
	if len(p.RequiredCriteria) == 0 || len(p.AllowedOutcomes) == 0 || len(p.AllowedJobStatuses) == 0 {
		return errors.New("release policy must explicitly name required criteria, allowed outcomes and job states")
	}
	for _, list := range [][]string{p.RequiredCriteria, p.AllowedOutcomes, p.AllowedJobStatuses} {
		seen := map[string]bool{}
		for _, value := range list {
			if !mail.Nonempty(value) || seen[value] {
				return errors.New("release policy lists must be nonempty and unique")
			}
			seen[value] = true
		}
	}
	for _, value := range p.AllowedOutcomes {
		if value != "pass" && value != "fail" && value != "mixed" {
			return errors.New("release policy outcome must be pass, fail or mixed; unevaluable cannot establish quality")
		}
	}
	for _, value := range p.AllowedJobStatuses {
		if value != store.Completed && value != store.Partial && value != store.WaitingInput {
			return errors.New("release policy job status must have a usable report: completed, partial or waiting_input")
		}
	}
	return nil
}

type CheckResult struct {
	Version     int      `json:"version"`
	ProposalID  string   `json:"proposal_id"`
	Name        string   `json:"name"`
	Outcome     string   `json:"outcome"`
	Actor       string   `json:"actor"`
	ActorKind   string   `json:"actor_kind"`
	Reason      string   `json:"reason"`
	EvidenceIDs []string `json:"evidence_ids"`
}

func (s Service) validateCheck(ctx context.Context, check CheckResult) error {
	if check.Version != Version || !mail.Nonempty(check.Actor) || !validActor(check.ActorKind) || !mail.Nonempty(check.Reason) || !outcome(check.Outcome) || len(check.EvidenceIDs) == 0 {
		return errors.New("check needs actor, outcome, reason and preserved evidence")
	}
	var proposal Proposal
	if err := s.load(ctx, check.ProposalID, "proposal", &proposal); err != nil {
		return err
	}
	if !slices.Contains(proposal.RequiredChecks, check.Name) {
		return errors.New("check is not declared by the proposal")
	}
	for _, id := range check.EvidenceIDs {
		r, err := s.Store.Record(ctx, id)
		if err != nil {
			return err
		}
		if _, err = s.Store.ReadRecord(r, s.Config.Limits.MaxArtifactBytes); err != nil {
			return err
		}
	}
	return nil
}

type ReleaseOptions struct {
	PolicyID string   `json:"policy_id"`
	CheckIDs []string `json:"check_ids"`
}

type ReleaseAssessment struct {
	Version       int            `json:"version"`
	ProposalID    string         `json:"proposal_id"`
	ComparisonID  string         `json:"comparison_id"`
	Options       ReleaseOptions `json:"options"`
	Eligible      bool           `json:"eligible"`
	Reasons       []string       `json:"reasons"`
	EvaluationIDs []string       `json:"evaluation_ids"`
}

func (s Service) Assess(ctx context.Context, proposalID, comparisonID string, options ReleaseOptions) (store.Record, ReleaseAssessment, error) {
	a := ReleaseAssessment{Version: Version, ProposalID: proposalID, ComparisonID: comparisonID, Options: options, Reasons: []string{}, EvaluationIDs: []string{}}
	var proposal Proposal
	var policy ReleasePolicy
	var comparison Comparison
	if options.PolicyID == "" {
		return store.Record{}, a, errors.New("select an explicit release policy with --policy; comparison alone is not adoption evidence")
	}
	if err := s.load(ctx, proposalID, "proposal", &proposal); err != nil {
		return store.Record{}, a, err
	}
	if err := s.load(ctx, options.PolicyID, "release_policy", &policy); err != nil {
		return store.Record{}, a, err
	}
	if err := policy.Validate(); err != nil {
		return store.Record{}, a, err
	}
	if err := s.load(ctx, comparisonID, "comparison", &comparison); err != nil {
		return store.Record{}, a, err
	}
	group := proposalWorkgroup(proposal)
	if policy.Workgroup != group {
		return store.Record{}, a, errors.New("release policy belongs to another workgroup")
	}
	_, active, err := workgroup.ActiveFor(s.Store.Root, group, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, a, err
	}
	if active != proposal.BaseDigest {
		a.Reasons = append(a.Reasons, "active controls changed since this proposal")
	}
	if policy.ReviewStatus != "human_reviewed" {
		a.Reasons = append(a.Reasons, "release policy has not been reviewed by a human")
	}
	if !comparison.Comparable || len(comparison.Runs) != 2 {
		a.Reasons = append(a.Reasons, "comparison is not a comparable baseline/candidate pair")
	}
	var baseline, candidate *RunEvidence
	for i := range comparison.Runs {
		run := &comparison.Runs[i]
		if run.Job.Workgroup != group || run.Manifest == nil {
			continue
		}
		if run.Manifest.WorkgroupDigest == proposal.BaseDigest {
			baseline = run
		}
		if run.Manifest.WorkgroupDigest == proposal.CandidateDigest {
			candidate = run
		}
	}
	if baseline == nil || candidate == nil {
		a.Reasons = append(a.Reasons, "comparison does not cover this proposal's exact baseline and candidate")
	} else {
		for _, run := range []*RunEvidence{baseline, candidate} {
			if run.SelectedEvaluationID == "" {
				a.Reasons = append(a.Reasons, "comparison has no explicit evaluation selection; create a new comparison")
				continue
			}
			var judgment Evaluation
			if err = s.load(ctx, run.SelectedEvaluationID, "evaluation", &judgment); err != nil {
				return store.Record{}, a, err
			}
			if judgment.JobID != run.Job.ID || judgment.AttemptID != run.Job.CurrentAttempt {
				return store.Record{}, a, errors.New("comparison evaluation ownership mismatch")
			}
			a.EvaluationIDs = append(a.EvaluationIDs, run.SelectedEvaluationID)
			run.SelectedEvaluation = &judgment
			current, e := s.Store.Job(ctx, run.Job.ID)
			if e != nil {
				return store.Record{}, a, e
			}
			if current.CurrentAttempt != run.Job.CurrentAttempt || current.Status != run.Job.Status {
				a.Reasons = append(a.Reasons, "compared job changed; recreate the comparison")
			}
			if !policy.AllowEvaluationLimitations && len(judgment.Limitations) != 0 {
				a.Reasons = append(a.Reasons, "selected evaluation has unresolved limitations: "+run.SelectedEvaluationID)
			}
			var evaluationCase Case
			var rubric Rubric
			if err = s.load(ctx, judgment.CaseID, "case", &evaluationCase); err != nil {
				return store.Record{}, a, err
			}
			if err = s.load(ctx, judgment.RubricID, "rubric", &rubric); err != nil {
				return store.Record{}, a, err
			}
			if policy.RequireReviewedExpectations && (evaluationCase.ReviewStatus != "human_reviewed" || rubric.ReviewStatus != "human_reviewed") {
				a.Reasons = append(a.Reasons, "selected expectations or rubric have not been reviewed by a human")
			}
			if policy.RequireConfirmation && (evaluationCase.Purpose != "confirmation" || evaluationCase.Exposure != "withheld") {
				a.Reasons = append(a.Reasons, "selected case lacks declared independent confirmation provenance")
			}
			if policy.RequireIsolatedAI && judgment.ReviewerKind == "ai" {
				if judgment.ReviewExecutionID == "" {
					a.Reasons = append(a.Reasons, "AI evaluation has no host-recorded isolated execution")
				} else if err = s.verifyReviewExecution(ctx, judgment); err != nil {
					return store.Record{}, a, err
				}
			}
		}
		if !slices.Contains(policy.AllowedJobStatuses, candidate.Job.Status) {
			a.Reasons = append(a.Reasons, "candidate job state is not allowed by the release policy")
		}
		if candidate.SelectedEvaluation != nil {
			if !slices.Contains(policy.AllowedOutcomes, candidate.SelectedEvaluation.Outcome) {
				a.Reasons = append(a.Reasons, "candidate evaluation outcome is not allowed by the release policy")
			}
			candidateJudgments := map[string]string{}
			for _, j := range candidate.SelectedEvaluation.Judgments {
				candidateJudgments[j.CriterionID] = j.Outcome
			}
			for _, criterion := range policy.RequiredCriteria {
				if candidateJudgments[criterion] != "pass" {
					a.Reasons = append(a.Reasons, "required criterion did not pass: "+criterion)
				}
			}
			if policy.RejectRegressions && baseline.SelectedEvaluation != nil {
				for _, j := range baseline.SelectedEvaluation.Judgments {
					if j.Outcome == "pass" && candidateJudgments[j.CriterionID] != "pass" {
						a.Reasons = append(a.Reasons, "previously passing criterion regressed: "+j.CriterionID)
					}
				}
			}
		}
	}
	checks := map[string]bool{}
	for _, id := range options.CheckIDs {
		var check CheckResult
		if err = s.load(ctx, id, "check", &check); err != nil {
			return store.Record{}, a, err
		}
		if check.ProposalID != proposalID || checks[check.Name] {
			return store.Record{}, a, errors.New("selected checks must belong to the proposal and name each check once")
		}
		if err = s.validateCheck(ctx, check); err != nil {
			return store.Record{}, a, err
		}
		checks[check.Name] = true
		if check.Outcome != "pass" {
			a.Reasons = append(a.Reasons, fmt.Sprintf("required check %s is %s", check.Name, check.Outcome))
		}
	}
	for _, name := range proposal.RequiredChecks {
		if !checks[name] {
			a.Reasons = append(a.Reasons, "required check has no selected result: "+name)
		}
	}
	a.Eligible = len(a.Reasons) == 0
	record, err := s.Store.PutRecord(ctx, "release_assessment", proposalID, a, s.Config.Limits.MaxArtifactBytes)
	return record, a, err
}
