package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"

	"chunsu/internal/files"
)

type Acquisition struct {
	ID           string `json:"id"`
	ConnectionID string `json:"connection_id"`
	ParentID     string `json:"parent_id"`
	Status       string `json:"status"`
	AsOf         string `json:"as_of"`
	NotBefore    int64  `json:"not_before"`
	Path         string `json:"path"`
	Digest       string `json:"digest"`
	Bytes        int64  `json:"bytes"`
	JobID        string `json:"job_id"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

const acquisitionColumns = "id,connection_id,parent_id,status,as_of,not_before,path,digest,bytes,job_id,created_at,updated_at"
const legacyGmailAcquisitionVersion = 1

func scanAcquisition(row scanner) (Acquisition, error) {
	var a Acquisition
	err := row.Scan(&a.ID, &a.ConnectionID, &a.ParentID, &a.Status, &a.AsOf, &a.NotBefore, &a.Path, &a.Digest, &a.Bytes, &a.JobID, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}
func (s *Store) Acquisition(ctx context.Context, id string) (Acquisition, error) {
	return scanAcquisition(s.DB.QueryRowContext(ctx, "SELECT "+acquisitionColumns+" FROM acquisitions WHERE id=?", id))
}

func (s *Store) Continuation(ctx context.Context, parent string) (Acquisition, error) {
	return scanAcquisition(s.DB.QueryRowContext(ctx, "SELECT "+acquisitionColumns+" FROM acquisitions WHERE parent_id=?", parent))
}
func (s *Store) Acquisitions(ctx context.Context, connection string) ([]Acquisition, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+acquisitionColumns+" FROM acquisitions WHERE (?='' OR connection_id=?) ORDER BY created_at DESC,id", connection, connection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Acquisition{}
	for rows.Next() {
		a, e := scanAcquisition(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AcquisitionsByProvider also recognizes legacy Gmail checkpoints, whose policy
// contains a query before a snapshot exists. Unknown or damaged records must not
// silently appear as a successful empty listing for a known provider.
func (s *Store) AcquisitionsByProvider(ctx context.Context, provider string, limit int64) ([]Acquisition, error) {
	all, err := s.Acquisitions(ctx, "")
	if err != nil {
		return nil, err
	}
	out := []Acquisition{}
	for _, a := range all {
		data, err := s.ReadAcquisition(a, limit)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Version  int    `json:"version"`
			Provider string `json:"provider"`
			Policy   struct {
				Query *string `json:"query"`
			} `json:"policy"`
			Snapshot struct {
				Origin struct {
					Provider string `json:"provider"`
				} `json:"origin"`
			} `json:"snapshot"`
		}
		if err = json.Unmarshal(data, &envelope); err != nil {
			return nil, errors.New("invalid acquisition provider envelope")
		}
		kind := envelope.Provider
		if kind == "" && envelope.Version == legacyGmailAcquisitionVersion && (envelope.Snapshot.Origin.Provider == "gmail" || (envelope.Policy.Query != nil && *envelope.Policy.Query != "")) {
			kind = "gmail"
		}
		if kind == "" {
			return nil, errors.New("acquisition provider is unknown")
		}
		if kind == provider {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *Store) SaveAcquisition(ctx context.Context, a Acquisition, payload any, limit int64) (Acquisition, error) {
	if !files.ValidID(a.ID) || !files.ValidID(a.ConnectionID) || (a.ParentID != "" && !files.ValidID(a.ParentID)) {
		return a, errors.New("invalid acquisition identity")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return a, err
	}
	if int64(len(data)) > limit {
		return a, errors.New("acquisition state exceeds configured byte limit")
	}
	if a.CreatedAt == 0 {
		a.CreatedAt = now()
	}
	a.UpdatedAt = now()
	a.Digest = files.Digest(data)
	a.Bytes = int64(len(data))
	a.Path = filepath.ToSlash(filepath.Join("state", "acquisitions", a.ID, files.ID()+".json"))
	if err = files.Write(s.Root, a.Path, data, false); err != nil {
		return a, err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO acquisitions("+acquisitionColumns+") VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,not_before=excluded.not_before,path=excluded.path,digest=excluded.digest,bytes=excluded.bytes,job_id=excluded.job_id,updated_at=excluded.updated_at", a.ID, a.ConnectionID, a.ParentID, a.Status, a.AsOf, a.NotBefore, a.Path, a.Digest, a.Bytes, a.JobID, a.CreatedAt, a.UpdatedAt)
	return a, err
}
func (s *Store) ReadAcquisition(a Acquisition, limit int64) ([]byte, error) {
	b, err := files.Read(s.Root, a.Path, limit)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != a.Bytes || files.Digest(b) != a.Digest {
		return nil, errors.New("acquisition state integrity mismatch")
	}
	return b, nil
}
func (s *Store) JobForAcquisition(ctx context.Context, id string) (Job, error) {
	return scanJob(s.DB.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM jobs WHERE json_extract(request_json,'$.acquisition_id')=?", id))
}
func (s *Store) Covered(ctx context.Context, connection, source, fingerprint string) (bool, error) {
	var stored string
	err := s.DB.QueryRowContext(ctx, "SELECT fingerprint FROM source_coverage WHERE connection_id=? AND source_id=?", connection, source).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil && stored == fingerprint, err
}
func (s *Store) RecordCoverage(ctx context.Context, connection, job string, fingerprints map[string]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	var request string
	if err = tx.QueryRowContext(ctx, "SELECT status,request_json FROM jobs WHERE id=?", job).Scan(&status, &request); err != nil {
		return err
	}
	if (Job{Request: json.RawMessage(request)}).IsExperiment() {
		return errors.New("candidate experiments cannot advance production source coverage")
	}
	if status != Completed && status != Partial {
		return errors.New("only an available validated report can advance source coverage")
	}
	for source, fingerprint := range fingerprints {
		if _, err = tx.ExecContext(ctx, "INSERT INTO source_coverage(connection_id,source_id,fingerprint,job_id,reported_at) VALUES(?,?,?,?,?) ON CONFLICT(connection_id,source_id) DO UPDATE SET fingerprint=excluded.fingerprint,job_id=excluded.job_id,reported_at=excluded.reported_at", connection, source, fingerprint, job, now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
