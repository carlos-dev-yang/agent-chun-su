package retention

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
)

const Version = 1
const Planned = "planned"
const Applying = "applying"
const Completed = "completed"
const Limitation = "Only this job's run directory and private request/event details are removed. Acquisition checkpoints, evaluations, proposals, source-coverage metadata, backups and external copies remain. Reproduction of this run will no longer be available. This is logical deletion, not secure media erasure."

type Entry struct {
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}
type Plan struct {
	Version      int              `json:"version"`
	ID           string           `json:"id"`
	JobID        string           `json:"job_id"`
	JobStatus    string           `json:"job_status"`
	JobUpdatedAt int64            `json:"job_updated_at"`
	CreatedAt    string           `json:"created_at"`
	Files        map[string]Entry `json:"files"`
	TotalBytes   int64            `json:"total_bytes"`
	Limitation   string           `json:"limitation"`
}
type Operation struct {
	ID         string `json:"id"`
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	PlanPath   string `json:"plan_path"`
	PlanDigest string `json:"plan_digest"`
}

func inventory(ctx context.Context, root, job string, limit int64) (map[string]Entry, int64, error) {
	if !files.ValidID(job) {
		return nil, 0, errors.New("invalid job identity")
	}
	entries := map[string]Entry{}
	var total int64
	err := filepath.WalkDir(filepath.Join(root, "runs", job), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("retention refuses symbolic links")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("retention refuses non-regular files")
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, n, err := files.HashFile(root, name, limit-total)
		if err != nil {
			return err
		}
		entries[filepath.ToSlash(name)] = Entry{Digest: digest, Bytes: n}
		total += n
		return nil
	})
	return entries, total, err
}

func Create(ctx context.Context, s *store.Store, c config.Config, jobID string) (Plan, error) {
	var p Plan
	j, err := s.Job(ctx, jobID)
	if err != nil {
		return p, err
	}
	switch j.Status {
	case store.Completed, store.Partial, store.Failed, store.Cancelled:
	default:
		return p, errors.New("finish or cancel pending work before planning retention")
	}
	entries, total, err := inventory(ctx, s.Root, j.ID, c.Limits.MaxBackupBytes)
	if err != nil {
		return p, err
	}
	p = Plan{Version: Version, ID: files.ID(), JobID: j.ID, JobStatus: j.Status, JobUpdatedAt: j.UpdatedAt, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Files: entries, TotalBytes: total, Limitation: Limitation}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return p, err
	}
	if int64(len(b)) > c.Limits.MaxEvidenceBytes {
		return p, errors.New("retention manifest exceeds byte limit")
	}
	path := filepath.ToSlash(filepath.Join("state", "retention", p.ID+".json"))
	if err = files.Write(s.Root, path, b, false); err != nil {
		return p, err
	}
	stamp := time.Now().UnixMilli()
	_, err = s.DB.ExecContext(ctx, "INSERT INTO retention_operations(id,job_id,status,plan_path,plan_digest,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", p.ID, j.ID, Planned, path, files.Digest(b), stamp, stamp)
	return p, err
}

func Read(ctx context.Context, s *store.Store, c config.Config, id string) (Operation, Plan, error) {
	var op Operation
	var p Plan
	err := s.DB.QueryRowContext(ctx, "SELECT id,job_id,status,plan_path,plan_digest FROM retention_operations WHERE id=?", id).Scan(&op.ID, &op.JobID, &op.Status, &op.PlanPath, &op.PlanDigest)
	if err != nil {
		return op, p, err
	}
	b, err := files.Read(s.Root, op.PlanPath, c.Limits.MaxEvidenceBytes)
	if err != nil {
		return op, p, err
	}
	if files.Digest(b) != op.PlanDigest {
		return op, p, errors.New("retention plan integrity mismatch")
	}
	if err = mail.Decode(b, &p); err != nil {
		return op, p, err
	}
	if p.Version != Version || p.ID != op.ID || p.JobID != op.JobID || !files.ValidID(p.JobID) {
		return op, p, errors.New("invalid retention plan binding")
	}
	return op, p, nil
}

func Apply(ctx context.Context, s *store.Store, c config.Config, id string) (Operation, error) {
	op, p, err := Read(ctx, s, c, id)
	if err != nil {
		return op, err
	}
	if op.Status == Completed {
		return op, nil
	}
	if op.Status != Planned && op.Status != Applying {
		return op, errors.New("unsupported retention state")
	}
	j, err := s.Job(ctx, p.JobID)
	if err != nil {
		return op, err
	}
	if op.Status == Planned {
		if j.Status != p.JobStatus || j.UpdatedAt != p.JobUpdatedAt {
			return op, errors.New("job changed since review; create a new retention plan")
		}
		current, total, err := inventory(ctx, s.Root, j.ID, c.Limits.MaxBackupBytes)
		if err != nil {
			return op, err
		}
		if total != p.TotalBytes || !reflect.DeepEqual(current, p.Files) {
			return op, errors.New("run contents changed since review; create a new plan")
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return op, err
		}
		defer tx.Rollback()
		stamp := time.Now().UnixMilli()
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status=?,updated_at=?,diagnostic=? WHERE id=?", store.Retiring, stamp, "explicit retention operation in progress", j.ID); err != nil {
			return op, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE artifacts SET content_state=? WHERE job_id=?", store.Retiring, j.ID); err != nil {
			return op, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE retention_operations SET status=?,updated_at=? WHERE id=?", Applying, stamp, op.ID); err != nil {
			return op, err
		}
		if err = tx.Commit(); err != nil {
			return op, err
		}
		op.Status = Applying
	} else if j.Status != store.Retiring {
		return op, errors.New("interrupted retention job has an incompatible state")
	}
	if err = files.RemoveTree(s.Root, filepath.Join("runs", p.JobID)); err != nil {
		return op, err
	}
	// Keep admission identity so the same Gmail acquisition is not queued again.
	var request map[string]json.RawMessage
	if err = json.Unmarshal(j.Request, &request); err != nil {
		return op, err
	}
	minimal := map[string]json.RawMessage{}
	for _, key := range []string{"origin", "connection_id", "acquisition_id"} {
		if v, ok := request[key]; ok {
			minimal[key] = v
		}
	}
	b, err := json.Marshal(minimal)
	if err != nil {
		return op, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return op, err
	}
	defer tx.Rollback()
	stamp := time.Now().UnixMilli()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"UPDATE jobs SET status=?,updated_at=?,diagnostic=?,request_json=? WHERE id=?", []any{store.Purged, stamp, "run content explicitly purged; see retention plan", string(b), p.JobID}},
		{"UPDATE artifacts SET content_state=? WHERE job_id=?", []any{store.Purged, p.JobID}},
		{"UPDATE attempts SET diagnostic='' WHERE job_id=?", []any{p.JobID}},
		{"UPDATE events SET data_json='{}' WHERE job_id=?", []any{p.JobID}},
		{"UPDATE retention_operations SET status=?,updated_at=? WHERE id=?", []any{Completed, stamp, op.ID}},
	} {
		if _, err = tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return op, err
		}
	}
	event, _ := json.Marshal(map[string]string{"plan_id": op.ID, "plan_digest": op.PlanDigest, "limitation": Limitation})
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", p.JobID, j.CurrentAttempt, "content.purged", stamp, string(event)); err != nil {
		return op, err
	}
	if err = tx.Commit(); err != nil {
		return op, err
	}
	op.Status = Completed
	return op, nil
}

func Pending(ctx context.Context, s *store.Store) ([]Operation, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,job_id,status,plan_path,plan_digest FROM retention_operations ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operation{}
	for rows.Next() {
		var op Operation
		if err = rows.Scan(&op.ID, &op.JobID, &op.Status, &op.PlanPath, &op.PlanDigest); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

func ValidConfirmation(id, confirmation string) bool {
	return files.ValidID(id) && strings.TrimSpace(confirmation) == id
}
