package feedback

import (
	"context"
	"os"
	"testing"

	"chunsu/internal/config"
	"chunsu/internal/store"
)

func TestCompareDoesNotCountReportWaitingForInputAsFailed(t *testing.T) {
	ctx := context.Background()
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

	baseline, err := s.Submit(ctx, "comparison-test", []byte(`{}`), map[string]any{}, c.Limits.MaxArtifactBytes)
	if err != nil {
		t.Fatal(err)
	}
	baselineAttempt, err := s.StartAttempt(ctx, baseline.ID, "test", c.Limits.MaxAttempts)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttempt(ctx, baselineAttempt, store.Failed, "result contract failed", 0); err != nil {
		t.Fatal(err)
	}

	candidate, err := s.Submit(ctx, "comparison-test", []byte(`{}`), map[string]any{}, c.Limits.MaxArtifactBytes)
	if err != nil {
		t.Fatal(err)
	}
	candidateAttempt, err := s.StartAttempt(ctx, candidate.ID, "test", c.Limits.MaxAttempts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveArtifact(ctx, candidate.ID, candidateAttempt.ID, "report_markdown", []byte("# report"), c.Limits.MaxArtifactBytes); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttempt(ctx, candidateAttempt, store.WaitingInput, "", 0); err != nil {
		t.Fatal(err)
	}

	_, comparison, err := service.Compare(ctx, baseline.ID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.AttemptCount != 2 {
		t.Fatalf("attempt count = %d, want 2", comparison.AttemptCount)
	}
	if comparison.FailedOrInterruptedAttempts != 1 {
		t.Fatalf("failed or interrupted attempts = %d, want 1", comparison.FailedOrInterruptedAttempts)
	}
	attempts, err := s.Attempts(ctx, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Status != store.Failed {
		t.Fatalf("candidate attempt = %+v, want persisted failed attempt", attempts)
	}
	job, err := s.Job(ctx, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != store.WaitingInput || job.Diagnostic != "" {
		t.Fatalf("candidate job = %+v, want waiting_input without diagnostic", job)
	}
}
