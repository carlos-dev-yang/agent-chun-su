package feedback

import (
	"context"
	"encoding/json"
	"errors"

	"chunsu/internal/executor"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

type RunEvidence struct {
	Job                  store.Job          `json:"job"`
	Attempts             []store.Attempt    `json:"attempts"`
	Artifacts            []store.Artifact   `json:"artifacts"`
	Manifest             *workgroup.Package `json:"manifest"`
	ExecutorResult       *executor.Result   `json:"executor_result"`
	Evaluations          []Evaluation       `json:"evaluations"`
	EvaluationIDs        []string           `json:"evaluation_ids,omitempty"`
	SelectedEvaluationID string             `json:"selected_evaluation_id,omitempty"`
	SelectedEvaluation   *Evaluation        `json:"selected_evaluation,omitempty"`
	Gaps                 []string           `json:"gaps"`
}
type Comparison struct {
	Version                     int           `json:"version"`
	Runs                        []RunEvidence `json:"runs"`
	Comparable                  bool          `json:"comparable"`
	Limitations                 []string      `json:"limitations"`
	JobCount                    int           `json:"job_count"`
	AttemptCount                int           `json:"attempt_count"`
	FailedOrInterruptedAttempts int           `json:"failed_or_interrupted_attempts"`
	UnevaluatedRuns             int           `json:"unevaluated_runs"`
}

func (s Service) InspectRun(ctx context.Context, id string) (RunEvidence, error) {
	r := RunEvidence{Evaluations: []Evaluation{}, Gaps: []string{}}
	var err error
	r.Job, err = s.Store.Job(ctx, id)
	if err != nil {
		return r, err
	}
	r.Attempts, err = s.Store.Attempts(ctx, id)
	if err != nil {
		return r, err
	}
	r.Artifacts, err = s.Store.Artifacts(ctx, id)
	if err != nil {
		return r, err
	}
	for _, art := range r.Artifacts {
		if art.AttemptID != r.Job.CurrentAttempt {
			continue
		}
		if art.Kind != "package_manifest" && art.Kind != "executor_result" {
			continue
		}
		b, e := s.Store.ReadArtifact(art, s.Config.Limits.MaxArtifactBytes)
		if e != nil {
			r.Gaps = append(r.Gaps, "unavailable or damaged artifact: "+art.ID)
			continue
		}
		if art.Kind == "package_manifest" {
			var p workgroup.Package
			if e = json.Unmarshal(b, &p); e != nil {
				return r, e
			}
			r.Manifest = &p
		} else {
			var result executor.Result
			if e = json.Unmarshal(b, &result); e != nil {
				return r, e
			}
			r.ExecutorResult = &result
		}
	}
	records, err := s.Store.Records(ctx, "evaluation", id, store.DefaultRecordLimit)
	if err != nil {
		return r, err
	}
	for _, record := range records {
		var e Evaluation
		if err = s.load(ctx, record.ID, "evaluation", &e); err != nil {
			r.Gaps = append(r.Gaps, "unavailable evaluation: "+record.ID)
			continue
		}
		if e.AttemptID == r.Job.CurrentAttempt {
			r.Evaluations = append(r.Evaluations, e)
			r.EvaluationIDs = append(r.EvaluationIDs, record.ID)
		}
	}
	if r.Manifest == nil {
		r.Gaps = append(r.Gaps, "no pinned manifest for the selected attempt")
	}
	if r.ExecutorResult == nil {
		r.Gaps = append(r.Gaps, "no executor outcome for the selected attempt")
	}
	if len(records) == store.DefaultRecordLimit {
		r.Gaps = append(r.Gaps, "evaluation listing reached its limit")
	}
	return r, nil
}

func (s Service) Compare(ctx context.Context, left, right string) (store.Record, Comparison, error) {
	return s.CompareSelected(ctx, left, right, "", "")
}

// CompareSelected pins the actual judgment pair. A single available evaluation
// is unambiguous; multiple evaluations require an explicit human selection.
func (s Service) CompareSelected(ctx context.Context, left, right, leftEvaluation, rightEvaluation string) (store.Record, Comparison, error) {
	c := Comparison{Version: Version, Runs: []RunEvidence{}, Comparable: true, Limitations: []string{}, JobCount: 2}
	if left == right {
		return store.Record{}, c, errors.New("select two distinct runs")
	}
	selected := []string{leftEvaluation, rightEvaluation}
	for index, id := range []string{left, right} {
		r, err := s.InspectRun(ctx, id)
		if err != nil {
			return store.Record{}, c, err
		}
		if selected[index] == "" && len(r.EvaluationIDs) == 1 {
			selected[index] = r.EvaluationIDs[0]
		}
		if selected[index] != "" {
			var judgment Evaluation
			if err = s.load(ctx, selected[index], "evaluation", &judgment); err != nil {
				return store.Record{}, c, err
			}
			if judgment.JobID != id || judgment.AttemptID != r.Job.CurrentAttempt {
				return store.Record{}, c, errors.New("selected evaluation does not belong to this job's current attempt")
			}
			r.SelectedEvaluationID, r.SelectedEvaluation = selected[index], &judgment
		} else if len(r.Evaluations) > 1 {
			r.Gaps = append(r.Gaps, "multiple evaluations exist; select the exact evaluation ID for job "+id)
		}
		c.Runs = append(c.Runs, r)
		c.AttemptCount += len(r.Attempts)
		for _, a := range r.Attempts {
			if a.Status == store.Failed || a.Status == store.Interrupted || a.Status == store.Cancelled {
				if waitingForInputAfterReport(r, a) {
					continue
				}
				c.FailedOrInterruptedAttempts++
			}
		}
		if len(r.Evaluations) == 0 {
			c.UnevaluatedRuns++
		}
		c.Limitations = append(c.Limitations, r.Gaps...)
	}
	a, b := c.Runs[0], c.Runs[1]
	if a.Job.Workgroup != b.Job.Workgroup {
		return store.Record{}, c, errors.New("cannot compare runs from different workgroups")
	}
	if a.Manifest == nil || b.Manifest == nil {
		c.Comparable = false
	} else {
		if a.Manifest.Executor.Model == "" || b.Manifest.Executor.Model == "" {
			c.Limitations = append(c.Limitations, "executor model was not explicitly pinned; matching implicit defaults are not established")
		}
		if a.Manifest.InputDigest != b.Manifest.InputDigest {
			c.Limitations = append(c.Limitations, "inputs/as-of/history differ")
		}
		if a.Manifest.RequestDigest == "" || a.Manifest.RequestDigest != b.Manifest.RequestDigest {
			c.Limitations = append(c.Limitations, "human request context is missing or differs")
		}
		if a.Manifest.Mode != b.Manifest.Mode {
			c.Limitations = append(c.Limitations, "report modes differ")
		}
		x, _ := json.Marshal(a.Manifest.Limits)
		y, _ := json.Marshal(b.Manifest.Limits)
		if string(x) != string(y) {
			c.Limitations = append(c.Limitations, "operational budgets differ")
		}
		leftExecutor, rightExecutor := a.Manifest.Executor, b.Manifest.Executor
		// Every candidate Jira Skill needs its own completed synthetic boundary
		// proof. Its job ID identifies that proof, rather than the executor's
		// behavior, so it is deliberately excluded from same-input comparison.
		leftExecutor.LiveJiraValidationJobID = ""
		rightExecutor.LiveJiraValidationJobID = ""
		x, _ = json.Marshal(leftExecutor)
		y, _ = json.Marshal(rightExecutor)
		if string(x) != string(y) {
			c.Limitations = append(c.Limitations, "executor configuration differs")
		}
	}
	if a.ExecutorResult == nil || b.ExecutorResult == nil || a.ExecutorResult.Version == "" || a.ExecutorResult.Version != b.ExecutorResult.Version {
		c.Limitations = append(c.Limitations, "executor runtime versions are missing or differ")
	}
	if a.SelectedEvaluation == nil || b.SelectedEvaluation == nil {
		c.Limitations = append(c.Limitations, "one or both runs have no selected semantic evaluation")
	} else {
		x, y := *a.SelectedEvaluation, *b.SelectedEvaluation
		if x.RubricID != y.RubricID || x.CaseID != y.CaseID || x.Reviewer != y.Reviewer || x.ReviewerKind != y.ReviewerKind || x.Workgroup != y.Workgroup || x.EvaluatorSkillID == "" || y.EvaluatorSkillID == "" || x.EvaluatorSkillID != y.EvaluatorSkillID || x.EvaluatorSkillHash != y.EvaluatorSkillHash {
			c.Limitations = append(c.Limitations, "evaluation rules, cases or reviewers differ; comparable rejudgment is required")
		}
		if x.Outcome == "unevaluable" || y.Outcome == "unevaluable" {
			c.Limitations = append(c.Limitations, "at least one result is unevaluable")
		}
	}
	if len(c.Limitations) > 0 {
		c.Comparable = false
	}
	record, err := s.Store.PutRecord(ctx, "comparison", "", c, s.Config.Limits.MaxArtifactBytes)
	return record, c, err
}

func waitingForInputAfterReport(r RunEvidence, a store.Attempt) bool {
	if a.Status != store.Failed || a.ID != r.Job.CurrentAttempt || r.Job.Status != store.WaitingInput || r.Job.Diagnostic != "" {
		return false
	}
	for _, artifact := range r.Artifacts {
		if artifact.AttemptID == a.ID && artifact.Kind == "report_markdown" {
			return true
		}
	}
	return false
}
