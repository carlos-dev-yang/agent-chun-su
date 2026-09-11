package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"chunsu/internal/codereview"
	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/store"
)

const Version = 1

type Service struct {
	Store  *store.Store
	Config config.Config
}
type Criterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}
type Rubric struct {
	Version      int         `json:"version"`
	Name         string      `json:"name"`
	ReviewStatus string      `json:"review_status"`
	Reviewer     string      `json:"reviewer"`
	Criteria     []Criterion `json:"criteria"`
}
type Expectation struct {
	Statement string   `json:"statement"`
	SourceIDs []string `json:"source_ids"`
	Kind      string   `json:"kind"`
}
type Case struct {
	Version      int           `json:"version"`
	Name         string        `json:"name"`
	ReviewStatus string        `json:"review_status"`
	Reviewer     string        `json:"reviewer"`
	Purpose      string        `json:"purpose,omitempty"`
	Exposure     string        `json:"exposure,omitempty"`
	Snapshot     mail.Snapshot `json:"snapshot"`
	// Workgroup is explicit for new cases. An omitted value remains the
	// historical mail-review representation so existing cases retain both their
	// JSON shape and fingerprint.
	Workgroup    string            `json:"workgroup,omitempty"`
	JiraSnapshot *jira.ReportInput `json:"jira_snapshot,omitempty"`
	CodeSnapshot *codereview.Input `json:"code_snapshot,omitempty"`
	Expectations []Expectation     `json:"expectations"`
	Limitations  []string          `json:"limitations"`
}
type Judgment struct {
	CriterionID string   `json:"criterion_id"`
	Outcome     string   `json:"outcome"`
	Reason      string   `json:"reason"`
	SourceIDs   []string `json:"source_ids"`
}
type EvaluatorSkill struct {
	Version   int    `json:"version"`
	Name      string `json:"name"`
	Workgroup string `json:"workgroup"`
	Content   string `json:"content"`
}
type Evaluation struct {
	Version            int        `json:"version"`
	JobID              string     `json:"job_id,omitempty"`
	AttemptID          string     `json:"attempt_id"`
	ResultArtifactID   string     `json:"result_artifact_id"`
	RubricID           string     `json:"rubric_id"`
	CaseID             string     `json:"case_id"`
	Reviewer           string     `json:"reviewer"`
	ReviewerKind       string     `json:"reviewer_kind"`
	Outcome            string     `json:"outcome"`
	Judgments          []Judgment `json:"judgments"`
	Limitations        []string   `json:"limitations"`
	CheckingSeconds    *int       `json:"checking_seconds"`
	ExpectationsStatus string     `json:"expectations_status,omitempty"`
	Workgroup          string     `json:"workgroup,omitempty"`
	EvaluatorSkillID   string     `json:"evaluator_skill_id,omitempty"`
	EvaluatorSkillHash string     `json:"evaluator_skill_hash,omitempty"`
	ReviewExecutionID  string     `json:"review_execution_id,omitempty"`
}
type Finding struct {
	Version     int      `json:"version"`
	Author      string   `json:"author"`
	AuthorKind  string   `json:"author_kind"`
	JobIDs      []string `json:"job_ids"`
	RecordIDs   []string `json:"record_ids"`
	Observation string   `json:"observation"`
	Hypothesis  string   `json:"hypothesis"`
	Limitations []string `json:"limitations"`
}

func validReview(status, reviewer string) bool {
	return mail.Nonempty(reviewer) && (status == "draft" || status == "human_reviewed" || status == "validation")
}
func validActor(kind string) bool { return kind == "human" || kind == "ai" || kind == "validation" }
func outcome(s string) bool       { return s == "pass" || s == "fail" || s == "unknown" }
func fingerprint(snapshot mail.Snapshot) string {
	b, _ := json.Marshal(snapshot)
	return files.Digest(b)
}

func caseWorkgroup(c Case) string {
	if c.Workgroup == "" {
		return mail.Workgroup
	}
	return c.Workgroup
}

// caseFingerprint retains the legacy mail byte contract. New Jira cases bind
// the declared workgroup with their typed report input so equal source IDs from
// different domains cannot compare as the same input.
func caseFingerprint(c Case) string {
	if c.CodeSnapshot != nil {
		b, _ := json.Marshal(struct {
			Workgroup string            `json:"workgroup"`
			Snapshot  *codereview.Input `json:"snapshot"`
		}{caseWorkgroup(c), c.CodeSnapshot})
		return files.Digest(b)
	}
	if c.JiraSnapshot == nil {
		return fingerprint(c.Snapshot)
	}
	b, _ := json.Marshal(struct {
		Workgroup string            `json:"workgroup"`
		Snapshot  *jira.ReportInput `json:"snapshot"`
	}{caseWorkgroup(c), c.JiraSnapshot})
	return files.Digest(b)
}

func caseSources(c Case, limits config.Limits) (map[string]bool, error) {
	sources := map[string]bool{}
	if c.CodeSnapshot != nil {
		if caseWorkgroup(c) != codereview.Workgroup || c.JiraSnapshot != nil {
			return nil, errors.New("code case must contain only its code-review input")
		}
		b, _ := json.Marshal(c.CodeSnapshot)
		in, err := codereview.Parse(b, limits)
		if err != nil {
			return nil, err
		}
		for _, source := range in.Sources {
			sources[source.ID] = true
		}
		return sources, nil
	}
	if c.JiraSnapshot == nil {
		if caseWorkgroup(c) != mail.Workgroup {
			return nil, errors.New("non-mail case requires its typed workgroup input")
		}
		b, _ := json.Marshal(c.Snapshot)
		snapshot, err := mail.ParseSnapshot(b, limits)
		if err != nil {
			return nil, err
		}
		for _, m := range snapshot.Messages {
			sources[m.ID] = true
		}
		return sources, nil
	}
	if caseWorkgroup(c) != "jira-report" {
		return nil, errors.New("Jira case requires a jira-report input")
	}
	data, err := json.Marshal(c.JiraSnapshot)
	if err != nil {
		return nil, err
	}
	input, err := jira.ParseReportInput(data)
	if err != nil {
		return nil, fmt.Errorf("invalid Jira case input: %w", err)
	}
	for _, issue := range input.Snapshot.Issues {
		if issue.ID == "" || sources[issue.ID] {
			return nil, errors.New("Jira case contains an invalid or duplicate source")
		}
		sources[issue.ID] = true
	}
	return sources, nil
}

func validateEvaluatorSkill(skill EvaluatorSkill) error {
	lines := strings.Split(skill.Content, "\n")
	if len(lines) < 5 || strings.TrimSpace(lines[0]) != "---" {
		return errors.New("evaluator Skill requires name and description front matter")
	}
	name, description, end := "", "", -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			end = i
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			return errors.New("evaluator Skill front matter is invalid")
		}
		switch strings.TrimSpace(key) {
		case "name":
			if name != "" {
				return errors.New("evaluator Skill name is duplicated")
			}
			name = strings.TrimSpace(value)
		case "description":
			if description != "" {
				return errors.New("evaluator Skill description is duplicated")
			}
			description = strings.TrimSpace(value)
		default:
			return errors.New("evaluator Skill front matter has an unsupported field")
		}
	}
	if end < 0 || name == "" || description == "" || name != skill.Name || strings.TrimSpace(strings.Join(lines[end+1:], "\n")) == "" {
		return errors.New("evaluator Skill name, description and body must be present")
	}
	return nil
}

func (s Service) load(ctx context.Context, id, kind string, target any) error {
	r, err := s.Store.Record(ctx, id)
	if err != nil {
		return err
	}
	if r.Kind != kind {
		return fmt.Errorf("record %s is not a %s", id, kind)
	}
	data, err := s.Store.ReadRecord(r, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return err
	}
	return mail.Decode(data, target)
}

func (s Service) Add(ctx context.Context, kind, subject string, data []byte) (store.Record, error) {
	var payload any
	switch kind {
	case "review_criteria":
		var criteria ReviewCriteria
		if err := mail.Decode(data, &criteria); err != nil {
			return store.Record{}, err
		}
		if err := criteria.Validate(); err != nil {
			return store.Record{}, err
		}
		payload = criteria
	case "release_policy":
		var policy ReleasePolicy
		if err := mail.Decode(data, &policy); err != nil {
			return store.Record{}, err
		}
		if err := policy.Validate(); err != nil {
			return store.Record{}, err
		}
		payload = policy
	case "check":
		var check CheckResult
		if err := mail.Decode(data, &check); err != nil {
			return store.Record{}, err
		}
		if err := s.validateCheck(ctx, check); err != nil {
			return store.Record{}, err
		}
		subject, payload = check.ProposalID, check
	case "rubric":
		var r Rubric
		if err := mail.Decode(data, &r); err != nil {
			return store.Record{}, err
		}
		if r.Version != Version || !mail.Nonempty(r.Name) || !validReview(r.ReviewStatus, r.Reviewer) || len(r.Criteria) == 0 {
			return store.Record{}, errors.New("rubric requires version, name, review status and criteria")
		}
		seen := map[string]bool{}
		for _, c := range r.Criteria {
			if !mail.Nonempty(c.ID) || !mail.Nonempty(c.Description) || seen[c.ID] {
				return store.Record{}, errors.New("rubric criteria must be unique and described")
			}
			seen[c.ID] = true
		}
		payload = r
	case "case":
		var c Case
		if err := mail.Decode(data, &c); err != nil {
			return store.Record{}, err
		}
		if c.Version != Version || !mail.Nonempty(c.Name) || !validReview(c.ReviewStatus, c.Reviewer) || len(c.Expectations) == 0 || c.Limitations == nil {
			return store.Record{}, errors.New("case requires reviewed-status metadata, expectations and limitations")
		}
		if c.Purpose != "" && c.Purpose != "tuning" && c.Purpose != "regression" && c.Purpose != "confirmation" {
			return store.Record{}, errors.New("case purpose must be tuning, regression or confirmation")
		}
		if c.Exposure != "" && c.Exposure != "development" && c.Exposure != "withheld" {
			return store.Record{}, errors.New("case exposure must be development or withheld")
		}
		sources, err := caseSources(c, s.Config.Limits)
		if err != nil {
			return store.Record{}, err
		}
		for _, e := range c.Expectations {
			if !mail.Nonempty(e.Statement) || (e.Kind != "required" && e.Kind != "prohibited" && e.Kind != "acceptable_alternative") {
				return store.Record{}, errors.New("invalid expectation")
			}
			for _, id := range e.SourceIDs {
				if !sources[id] {
					return store.Record{}, errors.New("expectation references an unknown source")
				}
			}
		}
		payload = c
	case "evaluator_skill":
		var skill EvaluatorSkill
		if err := mail.Decode(data, &skill); err != nil {
			return store.Record{}, err
		}
		if skill.Version != Version || !mail.Nonempty(skill.Name) || !mail.Nonempty(skill.Workgroup) || !mail.Nonempty(skill.Content) {
			return store.Record{}, errors.New("evaluator Skill requires version, name, workgroup and exact content")
		}
		if err := validateEvaluatorSkill(skill); err != nil {
			return store.Record{}, err
		}
		payload = skill
	case "evaluation":
		var e Evaluation
		if err := mail.Decode(data, &e); err != nil {
			return store.Record{}, err
		}
		e.JobID = subject
		if e.Version != Version || !mail.Nonempty(e.Reviewer) || !validActor(e.ReviewerKind) || (e.Outcome != "pass" && e.Outcome != "fail" && e.Outcome != "mixed" && e.Outcome != "unevaluable") || e.Limitations == nil {
			return store.Record{}, errors.New("invalid evaluation metadata")
		}
		if e.CheckingSeconds != nil && *e.CheckingSeconds < 0 {
			return store.Record{}, errors.New("checking time cannot be negative")
		}
		j, err := s.Store.Job(ctx, subject)
		if err != nil {
			return store.Record{}, err
		}
		if e.AttemptID == "" {
			e.AttemptID = j.CurrentAttempt
		}
		attempts, err := s.Store.Attempts(ctx, subject)
		if err != nil {
			return store.Record{}, err
		}
		matched := false
		for _, a := range attempts {
			if a.ID == e.AttemptID {
				matched = true
			}
		}
		if !matched {
			return store.Record{}, errors.New("evaluation must identify an existing attempt")
		}
		if e.ResultArtifactID != "" {
			artifacts, err := s.Store.Artifacts(ctx, subject)
			if err != nil {
				return store.Record{}, err
			}
			matched = false
			for _, a := range artifacts {
				if a.ID == e.ResultArtifactID && a.AttemptID == e.AttemptID && (a.Kind == "raw_result" || a.Kind == "report_markdown") {
					_, err = s.Store.ReadArtifact(a, s.Config.Limits.MaxArtifactBytes)
					if err != nil {
						return store.Record{}, err
					}
					matched = true
				}
			}
			if !matched {
				return store.Record{}, errors.New("evaluation result reference does not belong to the selected attempt")
			}
		} else if e.Outcome != "unevaluable" {
			return store.Record{}, errors.New("an evaluable judgment needs a preserved result artifact")
		}
		var rubric Rubric
		if err = s.load(ctx, e.RubricID, "rubric", &rubric); err != nil {
			return store.Record{}, err
		}
		var c Case
		if err = s.load(ctx, e.CaseID, "case", &c); err != nil {
			return store.Record{}, err
		}
		if e.Workgroup == "" {
			e.Workgroup = caseWorkgroup(c)
		}
		if e.Workgroup != j.Workgroup || e.Workgroup != caseWorkgroup(c) {
			return store.Record{}, errors.New("evaluation job, case and declared workgroup must match")
		}
		if (e.EvaluatorSkillID == "") != (e.EvaluatorSkillHash == "") {
			return store.Record{}, errors.New("evaluation evaluator Skill pin is incomplete")
		}
		if e.EvaluatorSkillID == "" {
			// Historical JSON imports predate explicit evaluator Skills. Preserve
			// them as traceable legacy evaluations, but mark the missing pin so a
			// later comparison cannot present them as the same evaluation rule.
			e.Limitations = append(e.Limitations, "independent evaluator Skill was not pinned by this legacy import")
		} else {
			skillRecord, err := s.Store.Record(ctx, e.EvaluatorSkillID)
			if err != nil {
				return store.Record{}, err
			}
			if skillRecord.Kind != "evaluator_skill" {
				return store.Record{}, errors.New("evaluation evaluator pin is not an evaluator Skill")
			}
			var skill EvaluatorSkill
			if err = s.load(ctx, e.EvaluatorSkillID, "evaluator_skill", &skill); err != nil {
				return store.Record{}, err
			}
			if skill.Workgroup != e.Workgroup || files.Digest([]byte(skill.Content)) != e.EvaluatorSkillHash {
				return store.Record{}, errors.New("evaluation evaluator Skill content or workgroup differs from its pin")
			}
		}
		input, err := s.Store.InputArtifact(ctx, j)
		if err != nil {
			return store.Record{}, err
		}
		b, err := s.Store.ReadArtifact(input, s.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return store.Record{}, err
		}
		actual, err := s.caseForInput(j.Workgroup, b)
		if err != nil {
			return store.Record{}, err
		}
		if caseFingerprint(actual) != caseFingerprint(c) {
			return store.Record{}, errors.New("evaluation case input differs from the job snapshot")
		}
		expected := map[string]bool{}
		for _, c := range rubric.Criteria {
			expected[c.ID] = false
		}
		sources, err := caseSources(actual, s.Config.Limits)
		if err != nil {
			return store.Record{}, err
		}
		for _, j := range e.Judgments {
			used, exists := expected[j.CriterionID]
			if !exists || used || !outcome(j.Outcome) || !mail.Nonempty(j.Reason) {
				return store.Record{}, errors.New("invalid or duplicate criterion judgment")
			}
			expected[j.CriterionID] = true
			for _, id := range j.SourceIDs {
				if !sources[id] {
					return store.Record{}, errors.New("judgment references an unknown source")
				}
			}
		}
		for _, present := range expected {
			if !present {
				return store.Record{}, errors.New("every rubric criterion needs a judgment, including unknown outcomes")
			}
		}
		if e.Outcome == "pass" {
			for _, judgment := range e.Judgments {
				if judgment.Outcome != "pass" {
					return store.Record{}, errors.New("passing evaluation requires every criterion to pass")
				}
			}
		}
		e.ExpectationsStatus = c.ReviewStatus + "; rubric=" + rubric.ReviewStatus
		if e.ReviewExecutionID != "" {
			if err = s.verifyReviewExecution(ctx, e); err != nil {
				return store.Record{}, err
			}
		}
		payload = e
	case "feedback", "finding":
		var f Finding
		if err := mail.Decode(data, &f); err != nil {
			return store.Record{}, err
		}
		if f.Version != Version || !validActor(f.AuthorKind) || !mail.Nonempty(f.Author) || !mail.Nonempty(f.Observation) || f.Limitations == nil {
			return store.Record{}, errors.New("finding requires author, observation and limitations")
		}
		for _, id := range f.JobIDs {
			if _, err := s.Store.Job(ctx, id); err != nil {
				return store.Record{}, err
			}
		}
		for _, id := range f.RecordIDs {
			if _, err := s.Store.Record(ctx, id); err != nil {
				return store.Record{}, err
			}
		}
		if kind == "finding" && !mail.Nonempty(f.Hypothesis) {
			return store.Record{}, errors.New("optimization finding must distinguish its hypothesis from the observation")
		}
		payload = f
	default:
		return store.Record{}, errors.New("unsupported feedback import kind")
	}
	return s.Store.PutRecord(ctx, kind, subject, payload, s.Config.Limits.MaxArtifactBytes)
}
