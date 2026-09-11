package feedback

import (
	"context"
	"path/filepath"

	"chunsu/internal/executor"
	"chunsu/internal/store"
)

// RecoverReviews must hold the controller writer lock. Incomplete reviews are
// reconciled and recorded as interrupted; no judgments or actions are replayed.
func (s Service) RecoverReviews(ctx context.Context) (int, error) {
	count := 0
	for {
		records, err := s.Store.PendingRecords(ctx, "review_request", "review_execution", store.DefaultRecordLimit)
		if err != nil {
			return count, err
		}
		if len(records) == 0 {
			return count, nil
		}
		for _, record := range records {
			var request ReviewRequest
			if err = s.load(ctx, record.ID, "review_request", &request); err != nil {
				return count, err
			}
			if err = executor.Reconcile(ctx, s.Store.Root, filepath.Join(request.PackagePath, executor.ProcessFile), s.Config.Limits); err != nil {
				return count, err
			}
			interrupted := ReviewExecution{Version: Version, RequestID: record.ID, Outcome: "interrupted", Executor: executor.Result{ExitCode: -1, Outcome: "interrupted"}, Diagnostic: "controller stopped before recording a completed review; inspect preserved evidence and explicitly request another review if needed"}
			if _, err = s.Store.PutRecord(ctx, "review_execution", record.ID, interrupted, s.Config.Limits.MaxArtifactBytes); err != nil {
				return count, err
			}
			count++
		}
	}
}
