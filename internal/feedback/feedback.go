package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"chunsu/internal/config"
	"chunsu/internal/files"
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
	Snapshot     mail.Snapshot `json:"snapshot"`
	Expectations []Expectation `json:"expectations"`
	Limitations  []string      `json:"limitations"`
}
type Judgment struct {
	CriterionID string   `json:"criterion_id"`
	Outcome     string   `json:"outcome"`
	Reason      string   `json:"reason"`
	SourceIDs   []string `json:"source_ids"`
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
		b, _ := json.Marshal(c.Snapshot)
		snapshot, err := mail.ParseSnapshot(b, s.Config.Limits)
		if err != nil {
			return store.Record{}, err
		}
		sources := map[string]bool{}
		for _, m := range snapshot.Messages {
			sources[m.ID] = true
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
		input, err := s.Store.InputArtifact(ctx, j)
		if err != nil {
			return store.Record{}, err
		}
		b, err := s.Store.ReadArtifact(input, s.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return store.Record{}, err
		}
		snapshot, err := mail.ParseSnapshot(b, s.Config.Limits)
		if err != nil {
			return store.Record{}, err
		}
		if fingerprint(snapshot) != fingerprint(c.Snapshot) {
			return store.Record{}, errors.New("evaluation case input differs from the job snapshot")
		}
		expected := map[string]bool{}
		for _, c := range rubric.Criteria {
			expected[c.ID] = false
		}
		sources := map[string]bool{}
		for _, m := range snapshot.Messages {
			sources[m.ID] = true
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
		e.ExpectationsStatus = c.ReviewStatus + "; rubric=" + rubric.ReviewStatus
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
