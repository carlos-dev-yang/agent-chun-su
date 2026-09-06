package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"chunsu/internal/files"
)

const (
	Queued           = "queued"
	Running          = "running"
	RetryWait        = "retry_wait"
	WaitingInput     = "waiting_input"
	WaitingAuth      = "waiting_auth"
	Completed        = "completed"
	Partial          = "partial"
	Failed           = "failed"
	Cancelled        = "cancelled"
	Interrupted      = "interrupted"
	Purged           = "purged"
	Retiring         = "retiring"
	ContentAvailable = "available"
	RecoveryRequired = "interrupted attempt requires recovery review"
)

func (s *Store) StartAttempt(ctx context.Context, jobID, executor string, maxAttempts int) (Attempt, error) {
	var a Attempt
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=?", jobID))
	if err != nil {
		return a, err
	}
	if j.Status != Queued && j.Status != RetryWait {
		return a, fmt.Errorf("job is %s, not eligible to run", j.Status)
	}
	if j.NotBefore > now() {
		return a, errors.New("job is not yet eligible to retry")
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM attempts WHERE job_id=?", jobID).Scan(&count); err != nil {
		return a, err
	}
	if count >= maxAttempts {
		return a, errors.New("attempt budget exhausted; submit a separate experiment for changed inputs or controls")
	}
	a = Attempt{ID: files.ID(), JobID: jobID, Ordinal: count + 1, Status: Running, StartedAt: now(), Executor: executor}
	_, err = tx.ExecContext(ctx, "INSERT INTO attempts(id,job_id,ordinal,status,started_at,executor) VALUES(?,?,?,?,?,?)", a.ID, jobID, a.Ordinal, a.Status, a.StartedAt, executor)
	if err != nil {
		return a, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,current_attempt=?,updated_at=?,diagnostic='' WHERE id=?", Running, a.ID, now(), jobID)
	if err != nil {
		return a, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", jobID, a.ID, "attempt.started", now(), `{}`)
	if err != nil {
		return a, err
	}
	return a, tx.Commit()
}

func (s *Store) FinishAttempt(ctx context.Context, a Attempt, status, diagnostic string, notBefore int64) error {
	allowed := map[string]bool{Completed: true, Partial: true, Failed: true, Cancelled: true, WaitingInput: true, WaitingAuth: true, RetryWait: true}
	if !allowed[status] {
		return errors.New("invalid completion status")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=?,diagnostic=?,not_before=? WHERE id=? AND current_attempt=? AND status=?", status, now(), diagnostic, notBefore, a.JobID, a.ID, Running)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("stale attempt cannot update this job")
	}
	attemptStatus := Failed
	if status == Completed || status == Partial {
		attemptStatus = Completed
	}
	if status == Cancelled {
		attemptStatus = Cancelled
	}
	_, err = tx.ExecContext(ctx, "UPDATE attempts SET status=?,ended_at=?,diagnostic=? WHERE id=? AND status=?", attemptStatus, now(), diagnostic, a.ID, Running)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]string{"status": status, "diagnostic": diagnostic})
	_, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", a.JobID, a.ID, "attempt.finished", now(), string(b))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Cancel(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=?", id))
	if err != nil {
		return err
	}
	if j.Status == Completed || j.Status == Partial || j.Status == Cancelled || j.Status == Purged || j.Status == Retiring {
		return fmt.Errorf("job is already %s", j.Status)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=?,diagnostic=? WHERE id=?", Cancelled, now(), "cancelled by user", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE attempts SET status=?,ended_at=?,diagnostic=? WHERE job_id=? AND status=?", Cancelled, now(), "cancelled by user", id, Running); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", id, j.CurrentAttempt, "job.cancelled", now(), `{}`); err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverInterrupted is called only after obtaining exclusive controller ownership
// and reconciling any surviving executor process. It never declares success.
func (s *Store) RecoverInterrupted(ctx context.Context) (int, error) {
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, j := range jobs {
		if j.Status != Running {
			continue
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return count, err
		}
		_, err = tx.ExecContext(ctx, "UPDATE attempts SET status=?,ended_at=?,diagnostic=? WHERE id=? AND status=?", Interrupted, now(), "controller interrupted; inspect evidence before retry", j.CurrentAttempt, Running)
		if err == nil {
			_, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=?,diagnostic=? WHERE id=? AND status=?", WaitingInput, now(), RecoveryRequired, j.ID, Running)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", j.ID, j.CurrentAttempt, "attempt.interrupted", now(), `{}`)
		}
		if err != nil {
			tx.Rollback()
			return count, err
		}
		if err = tx.Commit(); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Store) Attempts(ctx context.Context, id string) ([]Attempt, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,job_id,ordinal,status,started_at,ended_at,executor,diagnostic FROM attempts WHERE job_id=? ORDER BY ordinal", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		var a Attempt
		if err = rows.Scan(&a.ID, &a.JobID, &a.Ordinal, &a.Status, &a.StartedAt, &a.EndedAt, &a.Executor, &a.Diagnostic); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
