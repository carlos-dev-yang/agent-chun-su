package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"chunsu/internal/audit"
	"chunsu/internal/files"
)

const DefaultRecordLimit = 1000

type Record struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	SubjectID string `json:"subject_id,omitempty"`
	Path      string `json:"path"`
	Digest    string `json:"digest"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) PutRecord(ctx context.Context, kind, subject string, payload any, limit int64) (Record, error) {
	var r Record
	switch kind {
	case "case", "rubric", "evaluation", "evaluator_skill", "feedback", "finding", "proposal", "comparison", "decision", "release_policy", "check", "release_assessment", "review_request", "review_execution", "review_criteria":
	default:
		return r, errors.New("unsupported record kind")
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return r, err
	}
	if int64(len(data)) > limit {
		return r, errors.New("record exceeds byte limit")
	}
	r = Record{ID: files.ID(), Kind: kind, SubjectID: subject, Digest: files.Digest(data), Bytes: int64(len(data)), CreatedAt: now()}
	r.Path = filepath.ToSlash(filepath.Join("evaluations", r.ID+".json"))
	if kind == "proposal" || kind == "decision" {
		r.Path = filepath.ToSlash(filepath.Join("proposals", r.ID+".json"))
	}
	if err = files.Write(s.Root, r.Path, data, false); err != nil {
		return r, err
	}
	if err = audit.Record(s.Root, "record.prepared", r.ID, r.Digest); err != nil {
		return r, err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO records(id,kind,subject_id,path,digest,bytes,created_at) VALUES(?,?,?,?,?,?,?)", r.ID, r.Kind, r.SubjectID, r.Path, r.Digest, r.Bytes, r.CreatedAt)
	return r, err
}
func scanRecord(row scanner) (Record, error) {
	var r Record
	err := row.Scan(&r.ID, &r.Kind, &r.SubjectID, &r.Path, &r.Digest, &r.Bytes, &r.CreatedAt)
	return r, err
}

const recordColumns = "id,kind,subject_id,path,digest,bytes,created_at"

func (s *Store) PendingRecords(ctx context.Context, kind, completionKind string, limit int) ([]Record, error) {
	if limit <= 0 {
		return nil, errors.New("record limit must be positive")
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+recordColumns+" FROM records AS requested WHERE kind=? AND NOT EXISTS (SELECT 1 FROM records AS completed WHERE completed.kind=? AND completed.subject_id=requested.id) ORDER BY created_at,id LIMIT ?", kind, completionKind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) Record(ctx context.Context, id string) (Record, error) {
	return scanRecord(s.DB.QueryRowContext(ctx, "SELECT "+recordColumns+" FROM records WHERE id=?", id))
}
func (s *Store) ReadRecord(r Record, limit int64) ([]byte, error) {
	b, err := files.Read(s.Root, r.Path, limit)
	if err != nil {
		return nil, err
	}
	if files.Digest(b) != r.Digest || int64(len(b)) != r.Bytes {
		return nil, errors.New("record integrity mismatch")
	}
	return b, nil
}
func (s *Store) Records(ctx context.Context, kind, subject string, limit int) ([]Record, error) {
	if limit <= 0 {
		return nil, errors.New("record limit must be positive")
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT "+recordColumns+" FROM records WHERE (?='' OR kind=?) AND (?='' OR subject_id=?) ORDER BY created_at DESC,id LIMIT ?", kind, kind, subject, subject, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		r, e := scanRecord(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Store) InputArtifact(ctx context.Context, j Job) (Artifact, error) {
	artifacts, err := s.Artifacts(ctx, j.ID)
	if err != nil {
		return Artifact{}, err
	}
	for _, a := range artifacts {
		if a.Kind == "input" && a.Path == j.InputRef {
			return a, nil
		}
	}
	return Artifact{}, errors.New("job input artifact is missing")
}
