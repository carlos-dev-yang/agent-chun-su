package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	_ "modernc.org/sqlite"
)

const SchemaVersion = 4
const DBRelative = "state/chunsu.db"

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	DB   *sql.DB
	Root string
}

type Job struct {
	ID             string          `json:"id"`
	Workgroup      string          `json:"workgroup"`
	InputRef       string          `json:"input_ref"`
	Status         string          `json:"status"`
	CurrentAttempt string          `json:"current_attempt,omitempty"`
	CreatedAt      int64           `json:"created_at"`
	UpdatedAt      int64           `json:"updated_at"`
	NotBefore      int64           `json:"not_before,omitempty"`
	Diagnostic     string          `json:"diagnostic,omitempty"`
	Request        json.RawMessage `json:"request"`
}

func (j Job) IsExperiment() bool {
	var request struct {
		ExperimentOf string `json:"experiment_of"`
	}
	return json.Unmarshal(j.Request, &request) != nil || request.ExperimentOf != ""
}

func (j Job) CanAdvanceCoverage() bool {
	var request struct {
		DelegatedFrom string `json:"delegated_from"`
	}
	return !j.IsExperiment() && json.Unmarshal(j.Request, &request) == nil && request.DelegatedFrom == ""
}

type Attempt struct {
	ID         string `json:"id"`
	JobID      string `json:"job_id"`
	Ordinal    int    `json:"ordinal"`
	Status     string `json:"status"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    int64  `json:"ended_at,omitempty"`
	Executor   string `json:"executor"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type Artifact struct {
	ID           string `json:"id"`
	JobID        string `json:"job_id"`
	AttemptID    string `json:"attempt_id,omitempty"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	Digest       string `json:"digest"`
	Bytes        int64  `json:"bytes"`
	CreatedAt    int64  `json:"created_at"`
	ContentState string `json:"content_state"`
}

type Event struct {
	Sequence  int64           `json:"sequence"`
	JobID     string          `json:"job_id"`
	AttemptID string          `json:"attempt_id,omitempty"`
	Kind      string          `json:"kind"`
	CreatedAt int64           `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

func Open(ctx context.Context, root string, c config.Config, initialize bool) (*Store, error) {
	p := filepath.Join(root, DBRelative)
	info, statErr := os.Lstat(p)
	if statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("database must be a regular managed file")
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	if !initialize && statErr != nil {
		return nil, errors.New("database is not initialized; run setup")
	}
	if initialize && errors.Is(statErr, os.ErrNotExist) {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_RDWR, files.FileMode)
		if err != nil {
			return nil, err
		}
		f.Close()
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout("+strconv.FormatInt((time.Duration(c.Limits.LockWaitSeconds)*time.Second).Milliseconds(), 10)+")")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, Root: root}
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err = s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func OpenReadOnly(ctx context.Context, root string) (*Store, error) {
	if err := files.RequirePrivateDir(root); err != nil {
		return nil, err
	}
	if err := files.RequirePrivateDir(filepath.Join(root, "state")); err != nil {
		return nil, err
	}
	info, err := os.Lstat(filepath.Join(root, DBRelative))
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("database must be a regular managed file")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(root, DBRelative))}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	var v int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil || v != SchemaVersion {
		db.Close()
		return nil, fmt.Errorf("unsupported database schema version %d", v)
	}
	return &Store{DB: db, Root: root}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.DB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version < 0 || version > SchemaVersion {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	for version < SchemaVersion {
		version++
		script, err := migrations.ReadFile(fmt.Sprintf("migrations/%03d_core.sql", version))
		if err != nil {
			return err
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(script)); err == nil {
			_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version))
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() int64 { return time.Now().UTC().UnixMilli() }

func (s *Store) Submit(ctx context.Context, group string, input []byte, request any, limit int64) (Job, error) {
	var j Job
	if group == "" {
		return j, errors.New("workgroup is required")
	}
	if limit <= 0 || int64(len(input)) > limit {
		return j, errors.New("input exceeds configured limit")
	}
	b, err := json.Marshal(request)
	if err != nil {
		return j, err
	}
	j = Job{ID: files.ID(), Workgroup: group, Status: "queued", CreatedAt: now(), UpdatedAt: now(), Request: b}
	j.InputRef = filepath.ToSlash(filepath.Join("runs", j.ID, "input.json"))
	if err = files.Write(s.Root, j.InputRef, input, false); err != nil {
		return j, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return j, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO jobs(id,workgroup,input_ref,status,created_at,updated_at,request_json) VALUES(?,?,?,?,?,?,?)", j.ID, j.Workgroup, j.InputRef, j.Status, j.CreatedAt, j.UpdatedAt, string(b))
	if err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO artifacts(id,job_id,kind,path,digest,bytes,created_at) VALUES(?,?,?,?,?,?,?)", files.ID(), j.ID, "input", j.InputRef, files.Digest(input), len(input), now()); err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,kind,created_at,data_json) VALUES(?,?,?,?)", j.ID, "job.queued", now(), `{}`); err != nil {
		return j, err
	}
	return j, tx.Commit()
}

const jobColumns = "id,workgroup,input_ref,status,current_attempt,created_at,updated_at,not_before,diagnostic,request_json"

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var j Job
	var raw string
	err := row.Scan(&j.ID, &j.Workgroup, &j.InputRef, &j.Status, &j.CurrentAttempt, &j.CreatedAt, &j.UpdatedAt, &j.NotBefore, &j.Diagnostic, &raw)
	j.Request = json.RawMessage(raw)
	return j, err
}
func (s *Store) Job(ctx context.Context, id string) (Job, error) {
	return scanJob(s.DB.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=?", id))
}
func (s *Store) Jobs(ctx context.Context) ([]Job, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+jobColumns+" FROM jobs ORDER BY created_at DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) AddEvent(ctx context.Context, job, attempt, kind string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", job, attempt, kind, now(), string(b))
	return err
}
func (s *Store) Events(ctx context.Context, id string) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT sequence,job_id,attempt_id,kind,created_at,data_json FROM events WHERE job_id=? ORDER BY sequence", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var v Event
		var b string
		if err = rows.Scan(&v.Sequence, &v.JobID, &v.AttemptID, &v.Kind, &v.CreatedAt, &b); err != nil {
			return nil, err
		}
		v.Data = json.RawMessage(b)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Artifacts(ctx context.Context, id string) ([]Artifact, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,job_id,attempt_id,kind,path,digest,bytes,created_at,content_state FROM artifacts WHERE job_id=? ORDER BY created_at,id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Artifact{}
	for rows.Next() {
		var v Artifact
		if err = rows.Scan(&v.ID, &v.JobID, &v.AttemptID, &v.Kind, &v.Path, &v.Digest, &v.Bytes, &v.CreatedAt, &v.ContentState); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveArtifact(ctx context.Context, job, attempt, kind string, data []byte, limit int64) (Artifact, error) {
	var a Artifact
	if !files.ValidID(job) || attempt != "" && !files.ValidID(attempt) {
		return a, errors.New("invalid host identifier")
	}
	if int64(len(data)) > limit {
		return a, errors.New("artifact exceeds configured limit")
	}
	a = Artifact{ID: files.ID(), JobID: job, AttemptID: attempt, Kind: kind, Digest: files.Digest(data), Bytes: int64(len(data)), CreatedAt: now(), ContentState: ContentAvailable}
	a.Path = filepath.ToSlash(filepath.Join("runs", job, "artifacts", a.ID))
	if strings.HasSuffix(kind, "_markdown") {
		a.Path += ".md"
	}
	if err := files.Write(s.Root, a.Path, data, false); err != nil {
		return a, err
	}
	_, err := s.DB.ExecContext(ctx, "INSERT INTO artifacts(id,job_id,attempt_id,kind,path,digest,bytes,created_at) VALUES(?,?,?,?,?,?,?,?)", a.ID, job, attempt, kind, a.Path, a.Digest, a.Bytes, a.CreatedAt)
	return a, err
}

func (s *Store) ReadArtifact(a Artifact, limit int64) ([]byte, error) {
	if a.ContentState != "" && a.ContentState != ContentAvailable {
		return nil, errors.New("artifact content was explicitly retired and is no longer available")
	}
	b, err := files.Read(s.Root, a.Path, limit)
	if err != nil {
		return nil, err
	}
	if files.Digest(b) != a.Digest || int64(len(b)) != a.Bytes {
		return nil, errors.New("artifact integrity check failed")
	}
	return b, nil
}
