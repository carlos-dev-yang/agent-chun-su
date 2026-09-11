package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

const Version = 1
const ManifestName = "backup-manifest.json"
const RestoreNote = "state/restore.json"

var roots = []string{config.FileName, "runs", "workgroups", "evaluations", "proposals", "reviews", "state/acquisitions", "state/connections", "state/retention", RestoreNote}

type Entry struct {
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}
type Manifest struct {
	Version       int              `json:"version"`
	SchemaVersion int              `json:"schema_version"`
	CreatedAt     string           `json:"created_at"`
	Files         map[string]Entry `json:"files"`
	TotalBytes    int64            `json:"total_bytes"`
	Credentials   string           `json:"credentials"`
}

func contained(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && filepath.IsLocal(rel)
}
func destinationPath(source, destination string) (string, error) {
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", errors.New("create the destination's parent directory first")
	}
	absolute = filepath.Join(parent, filepath.Base(absolute))
	if contained(source, absolute) || contained(absolute, source) {
		return "", errors.New("choose a separate destination outside the source data directory")
	}
	if err = os.Mkdir(absolute, files.DirMode); err != nil {
		return "", errors.New("destination must be a new dedicated directory")
	}
	return absolute, nil
}
func allowed(name string) bool {
	if !filepath.IsLocal(name) || name == "." {
		return false
	}
	if name == store.DBRelative {
		return true
	}
	for _, root := range roots {
		if name == root || strings.HasPrefix(filepath.ToSlash(name), filepath.ToSlash(root)+"/") {
			return true
		}
	}
	return false
}

func Create(ctx context.Context, s *store.Store, c config.Config, destination string) (Manifest, error) {
	manifest := Manifest{Version: Version, SchemaVersion: store.SchemaVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Files: map[string]Entry{}, Credentials: "Keychain values excluded; restore requires reconnection"}
	jobs, err := s.Jobs(ctx)
	if err != nil {
		return manifest, err
	}
	for _, j := range jobs {
		if j.Status == store.Running {
			return manifest, errors.New("wait for active work to finish before backup")
		}
	}
	destination, err = destinationPath(s.Root, destination)
	if err != nil {
		return manifest, err
	}
	if err = files.PrivateDir(filepath.Join(destination, "state")); err != nil {
		return manifest, err
	}
	// VACUUM INTO includes committed WAL contents in a consistent SQLite image.
	if _, err = s.DB.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(destination, store.DBRelative)); err != nil {
		return manifest, err
	}
	if err = os.Chmod(filepath.Join(destination, store.DBRelative), files.FileMode); err != nil {
		return manifest, err
	}
	add := func(name string) error {
		digest, n, err := files.HashFile(destination, name, c.Limits.MaxBackupBytes-manifest.TotalBytes)
		if err != nil {
			return err
		}
		manifest.Files[filepath.ToSlash(name)] = Entry{Digest: digest, Bytes: n}
		manifest.TotalBytes += n
		return nil
	}
	if err = add(store.DBRelative); err != nil {
		return manifest, err
	}
	for _, root := range roots {
		path := filepath.Join(s.Root, root)
		if _, e := os.Lstat(path); errors.Is(e, os.ErrNotExist) {
			continue
		} else if e != nil {
			return manifest, e
		}
		err = filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("backup refuses symbolic links")
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return errors.New("backup refuses non-regular runtime files")
			}
			if strings.HasPrefix(entry.Name(), ".staging-") {
				return nil
			}
			relative, e := filepath.Rel(s.Root, path)
			if e != nil {
				return e
			}
			if e = files.Copy(s.Root, relative, destination, c.Limits.MaxBackupBytes-manifest.TotalBytes); e != nil {
				return e
			}
			return add(relative)
		})
		if err != nil {
			return manifest, err
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return manifest, err
	}
	if int64(len(data)) > c.Limits.MaxEvidenceBytes {
		return manifest, errors.New("backup manifest exceeds configured byte limit")
	}
	return manifest, files.Write(destination, ManifestName, data, false)
}

func Verify(ctx context.Context, root string, limit int64) (Manifest, error) {
	var manifest Manifest
	data, err := files.Read(root, ManifestName, config.DefaultMaxEvidenceBytes)
	if err != nil {
		return manifest, err
	}
	if err = mail.Decode(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != Version || manifest.SchemaVersion < 1 || manifest.SchemaVersion > store.SchemaVersion {
		return manifest, errors.New("unsupported backup format or database version")
	}
	if _, ok := manifest.Files[store.DBRelative]; !ok {
		return manifest, errors.New("backup database is missing")
	}
	if _, ok := manifest.Files[config.FileName]; !ok {
		return manifest, errors.New("backup configuration is missing")
	}
	var total int64
	for name, entry := range manifest.Files {
		if ctx.Err() != nil {
			return manifest, ctx.Err()
		}
		if !allowed(name) || !files.ValidDigest(entry.Digest) || entry.Bytes < 0 {
			return manifest, errors.New("backup has an invalid file declaration")
		}
		digest, n, err := files.HashFile(root, name, limit-total)
		if err != nil {
			return manifest, err
		}
		if digest != entry.Digest || n != entry.Bytes {
			return manifest, errors.New("backup content integrity mismatch")
		}
		total += n
	}
	if total != manifest.TotalBytes {
		return manifest, errors.New("backup total does not match its manifest")
	}
	return manifest, nil
}

func Restore(ctx context.Context, source, destination string, limit int64) (string, error) {
	manifest, err := Verify(ctx, source, limit)
	if err != nil {
		return "", err
	}
	destination, err = destinationPath(source, destination)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(manifest.Files))
	for name := range manifest.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err = files.Copy(source, name, destination, limit); err != nil {
			return destination, err
		}
	}
	if _, err = config.Setup(destination); err != nil {
		return destination, err
	}
	c, err := config.Load(destination)
	if err != nil {
		return destination, err
	}
	c.Executor.LiveMailApproved = false
	c.Executor.LiveJiraApproved = false
	c.Executor.LiveJiraPolicyDigest = ""
	c.Executor.LiveJiraValidationJobID = ""
	if err = config.Save(destination, c); err != nil {
		return destination, err
	}
	s, err := store.Open(ctx, destination, c, false)
	if err != nil {
		return destination, err
	}
	defer s.Close()
	if err = VerifyReferences(ctx, s, c.Limits.MaxBackupBytes); err != nil {
		return destination, err
	}
	if _, _, err = workgroup.Active(destination, c.Limits.MaxArtifactBytes); err != nil {
		return destination, err
	}
	for name := range manifest.Files {
		if strings.HasPrefix(name, "state/connections/jira-cloud/") && strings.Count(strings.TrimSuffix(name, ".json"), "/") == 3 && strings.HasSuffix(name, ".json") {
			id := strings.TrimSuffix(filepath.Base(name), ".json")
			profile, e := jira.ReadProfile(destination, id, c)
			if e != nil {
				return destination, e
			}
			profile.Active = false
			profile.Secret = jira.SecretRef{}
			if e = jira.SaveProfile(destination, profile, true); e != nil {
				return destination, e
			}
			continue
		}
		if strings.HasPrefix(name, "state/connections/") && strings.Count(strings.TrimSuffix(name, ".json"), "/") == 2 && strings.HasSuffix(name, ".json") {
			id := strings.TrimSuffix(filepath.Base(name), ".json")
			connection, e := gmail.ReadConnection(destination, id, c)
			if e != nil {
				return destination, e
			}
			connection.Enabled = false
			connection.RefreshRef = files.ID()
			connection.ClientSecretRef = files.ID()
			connection.RetiredSecretRefs = nil
			if e = gmail.SaveConnection(destination, connection, true); e != nil {
				return destination, e
			}
		}
	}
	if _, err = s.DB.ExecContext(ctx, "UPDATE jobs SET status=?,diagnostic=? WHERE status IN (?,?,?)", store.WaitingInput, "restored data requires explicit resume after account/configuration review", store.Queued, store.RetryWait, store.Running); err != nil {
		return destination, err
	}
	if _, err = s.DB.ExecContext(ctx, "UPDATE schedules SET enabled=0"); err != nil {
		return destination, err
	}
	if err = s.SetPaused(ctx, true); err != nil {
		return destination, err
	}
	note, _ := json.Marshal(map[string]any{"version": Version, "restored_at": time.Now().UTC().Format(time.RFC3339Nano), "source_manifest": manifest, "connections_enabled": false, "user_review_required": true})
	if err = files.Write(destination, RestoreNote, note, true); err != nil {
		return destination, err
	}
	return destination, nil
}

func VerifyReferences(ctx context.Context, s *store.Store, limit int64) error {
	rows, err := s.DB.QueryContext(ctx, "SELECT path,digest,bytes FROM artifacts WHERE content_state='available' UNION ALL SELECT path,digest,bytes FROM records UNION ALL SELECT path,digest,bytes FROM acquisitions")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var path, digest string
		var n int64
		if err = rows.Scan(&path, &digest, &n); err != nil {
			return err
		}
		actual, bytes, e := files.HashFile(s.Root, path, limit)
		if e != nil {
			return e
		}
		if digest != actual || n != bytes {
			return errors.New("restored reference does not match its content")
		}
	}
	return rows.Err()
}
