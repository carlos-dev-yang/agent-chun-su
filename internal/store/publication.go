package store

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Store) CompletePublication(ctx context.Context, job, attempt, status, artifact string) error {
	if status != Completed && status != Partial && status != WaitingInput {
		return errors.New("invalid publication completion state")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=?", job))
	if err != nil {
		return err
	}
	if j.CurrentAttempt != attempt || j.Status != WaitingInput || !(j.Diagnostic == PublicationRequired || j.Diagnostic == RecoveryRequired) {
		return errors.New("job is not waiting for publication recovery")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=?,diagnostic='' WHERE id=?", status, now(), job); err != nil {
		return err
	}
	event, _ := json.Marshal(map[string]string{"artifact_id": artifact, "availability": "local", "acknowledgment": "unknown"})
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", job, attempt, "report.publication_recovered", now(), string(event)); err != nil {
		return err
	}
	return tx.Commit()
}
