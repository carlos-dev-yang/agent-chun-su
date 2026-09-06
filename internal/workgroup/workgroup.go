package workgroup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/workgroups"
)

const BundleVersion = 1
const ActivePath = "workgroups/mail-review/active.json"

type Bundle struct {
	Version int             `json:"version"`
	Guide   string          `json:"guide"`
	Schema  json.RawMessage `json:"schema"`
}
type Selection struct {
	Digest     string `json:"digest"`
	Reason     string `json:"reason"`
	DecisionID string `json:"decision_id,omitempty"`
}

func Default() (Bundle, error) {
	guide, err := workgroups.Assets.ReadFile("mail-review/guide.md")
	if err != nil {
		return Bundle{}, err
	}
	schema, err := workgroups.Assets.ReadFile("mail-review/report.schema.json")
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{Version: BundleVersion, Guide: string(guide), Schema: schema}, nil
}
func BundlePath(digest string) (string, error) {
	if len(digest) != 64 {
		return "", errors.New("invalid workgroup digest")
	}
	for _, c := range digest {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", errors.New("invalid workgroup digest")
		}
	}
	return filepath.ToSlash(filepath.Join("workgroups", mail.Workgroup, "versions", digest+".json")), nil
}
func Put(root string, b Bundle) (string, error) {
	if b.Version != BundleVersion || !mail.Nonempty(b.Guide) || !json.Valid(b.Schema) {
		return "", errors.New("invalid workgroup bundle")
	}
	if _, err := mail.CompileSchema(b.Schema); err != nil {
		return "", err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	digest := files.Digest(data)
	p, err := BundlePath(digest)
	if err != nil {
		return "", err
	}
	err = files.Write(root, p, data, false)
	if errors.Is(err, os.ErrExist) {
		existing, e := files.Read(root, p, int64(len(data)))
		if e != nil || files.Digest(existing) != digest {
			return "", errors.New("existing bundle failed integrity check")
		}
		return digest, nil
	}
	return digest, err
}
func Install(root string, limit int64) error {
	if _, err := files.Read(root, ActivePath, limit); err == nil {
		_, _, e := Active(root, limit)
		return e
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	b, err := Default()
	if err != nil {
		return err
	}
	digest, err := Put(root, b)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(Selection{Digest: digest, Reason: "initial built-in public guide; personal policy not yet reviewed"})
	return files.Write(root, ActivePath, data, false)
}
func Load(root, digest string, limit int64) (Bundle, error) {
	var b Bundle
	p, err := BundlePath(digest)
	if err != nil {
		return b, err
	}
	data, err := files.Read(root, p, limit)
	if err != nil {
		return b, err
	}
	if files.Digest(data) != digest {
		return b, errors.New("workgroup bundle integrity mismatch")
	}
	if err = mail.Decode(data, &b); err != nil {
		return b, err
	}
	if b.Version != BundleVersion {
		return b, errors.New("unsupported workgroup bundle version")
	}
	return b, nil
}
func Active(root string, limit int64) (Bundle, string, error) {
	var selected Selection
	data, err := files.Read(root, ActivePath, limit)
	if err != nil {
		return Bundle{}, "", err
	}
	if err = mail.Decode(data, &selected); err != nil {
		return Bundle{}, "", err
	}
	b, err := Load(root, selected.Digest, limit)
	return b, selected.Digest, err
}
