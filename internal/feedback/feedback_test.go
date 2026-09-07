package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"chunsu/internal/config"
	"chunsu/internal/mail"
	"chunsu/internal/store"
)

func TestAddEvaluationRejectsPassWithNonPassCriterion(t *testing.T) {
	tests := []struct {
		name            string
		outcome         string
		judgments       []Judgment
		includeArtifact bool
		wantError       bool
	}{
		{
			name:    "pass rejects a failed criterion",
			outcome: "pass",
			judgments: []Judgment{
				{CriterionID: "source", Outcome: "pass", Reason: "source verified"},
				{CriterionID: "meaning", Outcome: "fail", Reason: "summary contradicts source"},
			},
			includeArtifact: true,
			wantError:       true,
		},
		{
			name:    "pass rejects an unknown criterion",
			outcome: "pass",
			judgments: []Judgment{
				{CriterionID: "source", Outcome: "pass", Reason: "source verified"},
				{CriterionID: "meaning", Outcome: "unknown", Reason: "body unavailable"},
			},
			includeArtifact: true,
			wantError:       true,
		},
		{
			name:    "pass accepts every criterion passing",
			outcome: "pass",
			judgments: []Judgment{
				{CriterionID: "source", Outcome: "pass", Reason: "source verified"},
				{CriterionID: "meaning", Outcome: "pass", Reason: "summary matches source"},
			},
			includeArtifact: true,
		},
		{
			name:    "unevaluable permits observed pass criteria without a result reference",
			outcome: "unevaluable",
			judgments: []Judgment{
				{CriterionID: "source", Outcome: "pass", Reason: "source verified"},
				{CriterionID: "meaning", Outcome: "pass", Reason: "format observed before output was unavailable"},
			},
		},
		{
			name:    "fail behavior remains accepted",
			outcome: "fail",
			judgments: []Judgment{
				{CriterionID: "source", Outcome: "fail", Reason: "source omitted"},
				{CriterionID: "meaning", Outcome: "unknown", Reason: "body unavailable"},
			},
			includeArtifact: true,
		},
		{
			name:    "mixed behavior remains accepted",
			outcome: "mixed",
			judgments: []Judgment{
				{CriterionID: "source", Outcome: "unknown", Reason: "source unavailable"},
				{CriterionID: "meaning", Outcome: "unknown", Reason: "body unavailable"},
			},
			includeArtifact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			service, job, attempt, rubric, evaluationCase, result := evaluationFixture(t, ctx)
			baseline := evaluationFor(job, attempt, rubric, evaluationCase, "pass", []Judgment{
				{CriterionID: "source", Outcome: "pass", Reason: "source verified"},
				{CriterionID: "meaning", Outcome: "pass", Reason: "summary matches source"},
			}, result.ID)
			baselineData, err := json.Marshal(baseline)
			if err != nil {
				t.Fatal(err)
			}
			baselineRecord, err := service.Add(ctx, "evaluation", job.ID, baselineData)
			if err != nil {
				t.Fatal(err)
			}
			before, err := service.Store.ReadRecord(baselineRecord, service.Config.Limits.MaxArtifactBytes)
			if err != nil {
				t.Fatal(err)
			}

			resultID := ""
			if tt.includeArtifact {
				resultID = result.ID
			}
			candidate := evaluationFor(job, attempt, rubric, evaluationCase, tt.outcome, tt.judgments, resultID)
			candidateData, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			record, err := service.Add(ctx, "evaluation", job.ID, candidateData)
			if tt.wantError {
				if err == nil {
					t.Fatalf("Add accepted pass with non-pass criterion: %+v", candidate)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if record.Kind != "evaluation" {
					t.Fatalf("record kind = %q, want evaluation", record.Kind)
				}
			}

			afterRecord, err := service.Store.Record(ctx, baselineRecord.ID)
			if err != nil {
				t.Fatal(err)
			}
			after, err := service.Store.ReadRecord(afterRecord, service.Config.Limits.MaxArtifactBytes)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("existing evaluation record changed")
			}
			afterJob, err := service.Store.Job(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if afterJob.Status != store.Running || afterJob.CurrentAttempt != attempt.ID {
				t.Fatalf("evaluation changed operational state: status=%q attempt=%q", afterJob.Status, afterJob.CurrentAttempt)
			}
			records, err := service.Store.Records(ctx, "evaluation", job.ID, store.DefaultRecordLimit)
			if err != nil {
				t.Fatal(err)
			}
			wantRecords := 2
			if tt.wantError {
				wantRecords = 1
			}
			if len(records) != wantRecords {
				t.Fatalf("evaluation records = %d, want %d", len(records), wantRecords)
			}
		})
	}
}

func evaluationFor(job store.Job, attempt store.Attempt, rubric, evaluationCase store.Record, outcome string, judgments []Judgment, resultID string) Evaluation {
	return Evaluation{
		Version:          Version,
		AttemptID:        attempt.ID,
		ResultArtifactID: resultID,
		RubricID:         rubric.ID,
		CaseID:           evaluationCase.ID,
		Reviewer:         "test reviewer",
		ReviewerKind:     "validation",
		Outcome:          outcome,
		Judgments:        judgments,
		Limitations:      []string{},
	}
}

func evaluationFixture(t *testing.T, ctx context.Context) (Service, store.Job, store.Attempt, store.Record, store.Record, store.Artifact) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Setup(root); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(ctx, root, c, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	service := Service{Store: s, Config: c}
	snapshot := mail.Snapshot{
		Version:   mail.Version,
		Synthetic: true,
		AsOf:      "2026-09-07T00:00:00Z",
		Timezone:  "UTC",
		Collection: mail.Collection{
			Status: "complete",
			Errors: []string{},
		},
		Messages: []mail.Message{{
			ID:            "source-1",
			ThreadID:      "thread-1",
			ReceivedAt:    "2026-09-06T00:00:00Z",
			Scope:         mail.Target,
			ContentStatus: "complete",
			Attachments:   []mail.Attachment{},
		}},
		PriorInterpretations: []mail.Interpretation{},
	}
	input, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.Submit(ctx, mail.Workgroup, input, map[string]any{}, c.Limits.MaxArtifactBytes)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := s.StartAttempt(ctx, job.ID, "test", c.Limits.MaxAttempts)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.SaveArtifact(ctx, job.ID, attempt.ID, "raw_result", []byte(`{"status":"available"}`), c.Limits.MaxArtifactBytes)
	if err != nil {
		t.Fatal(err)
	}
	rubricData, err := json.Marshal(Rubric{Version: Version, Name: "test rubric", ReviewStatus: "validation", Reviewer: "test reviewer", Criteria: []Criterion{{ID: "source", Description: "source check"}, {ID: "meaning", Description: "meaning check"}}})
	if err != nil {
		t.Fatal(err)
	}
	rubric, err := service.Add(ctx, "rubric", "", rubricData)
	if err != nil {
		t.Fatal(err)
	}
	caseData, err := json.Marshal(Case{Version: Version, Name: "test case", ReviewStatus: "validation", Reviewer: "test reviewer", Snapshot: snapshot, Expectations: []Expectation{{Statement: "preserve source", SourceIDs: []string{"source-1"}, Kind: "required"}}, Limitations: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	evaluationCase, err := service.Add(ctx, "case", "", caseData)
	if err != nil {
		t.Fatal(err)
	}
	return service, job, attempt, rubric, evaluationCase, result
}
