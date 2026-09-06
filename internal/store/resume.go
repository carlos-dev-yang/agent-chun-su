package store

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Store) Resume(ctx context.Context, id, answer string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=?", id))
	if err != nil {
		return err
	}
	switch j.Status {
	case WaitingInput:
		if answer == "" {
			return errors.New("this job needs an answer or a recovery-review note")
		}
	case Failed, RetryWait, WaitingAuth, Cancelled:
	default:
		return errors.New("job is not eligible for explicit resume")
	}
	var request map[string]any
	if err = json.Unmarshal(j.Request, &request); err != nil {
		return err
	}
	if request == nil {
		request = map[string]any{}
	}
	if answer != "" {
		replies, _ := request["human_replies"].([]any)
		request["human_replies"] = append(replies, map[string]any{"attempt_id": j.CurrentAttempt, "answer": answer, "at": now()})
	}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=?,not_before=0,diagnostic='',request_json=? WHERE id=?", Queued, now(), string(data), id); err != nil {
		return err
	}
	event, _ := json.Marshal(map[string]any{"previous_status": j.Status, "has_answer": answer != ""})
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", id, j.CurrentAttempt, "job.resumed", now(), string(event)); err != nil {
		return err
	}
	return tx.Commit()
}
