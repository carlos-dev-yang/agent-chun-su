package workgroup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/workgroups"
)

const BundleVersion = 2
const ActivePath = "workgroups/mail-review/active.json"

type Bundle struct {
	Version   int             `json:"version"`
	Guide     string          `json:"guide"`
	Schema    json.RawMessage `json:"schema"`
	Workgroup string          `json:"workgroup,omitempty"`
	Skill     *Skill          `json:"skill,omitempty"`
}

type Selection struct {
	Digest     string `json:"digest"`
	Reason     string `json:"reason"`
	DecisionID string `json:"decision_id,omitempty"`
}

func Default() (Bundle, error) { return DefaultFor(mail.Workgroup) }

func DefaultFor(id string) (Bundle, error) {
	if !allowedWorkgroup(id) {
		return Bundle{}, errors.New("unsupported workgroup")
	}
	skill, err := readEmbeddedSkill(id)
	if err != nil {
		return Bundle{}, err
	}
	schema, err := workgroups.Assets.ReadFile(filepath.ToSlash(filepath.Join(id, "report.schema.json")))
	if err != nil {
		return Bundle{}, err
	}
	b := Bundle{Version: BundleVersion, Schema: schema, Workgroup: id, Skill: &skill}
	return b, b.Validate()
}

func BundlePath(digest string) (string, error) { return bundlePathFor(mail.Workgroup, digest) }

func bundlePathFor(id, digest string) (string, error) {
	if !allowedWorkgroup(id) {
		return "", errors.New("unsupported workgroup")
	}
	if !files.ValidDigest(digest) {
		return "", errors.New("invalid workgroup digest")
	}
	for _, c := range digest {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", errors.New("invalid workgroup digest")
		}
	}
	return filepath.ToSlash(filepath.Join("workgroups", id, "versions", digest+".json")), nil
}

func ActivePathFor(id string) (string, error) {
	if !allowedWorkgroup(id) {
		return "", errors.New("unsupported workgroup")
	}
	return filepath.ToSlash(filepath.Join("workgroups", id, "active.json")), nil
}

func Put(root string, b Bundle) (string, error) {
	if b.Version == 1 {
		if err := b.Validate(); err != nil {
			return "", err
		}
		skill, err := legacyMailSkill(b.Guide)
		if err != nil {
			return "", err
		}
		b = Bundle{Version: BundleVersion, Schema: b.Schema, Workgroup: mail.Workgroup, Skill: &skill}
	}
	if err := b.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	digest := files.Digest(data)
	p, err := bundlePathFor(b.Workgroup, digest)
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

func Install(root string, limit int64) error { return InstallFor(root, mail.Workgroup, limit) }

func InstallFor(root, id string, limit int64) error {
	activePath, err := ActivePathFor(id)
	if err != nil {
		return err
	}
	if _, err := files.Read(root, activePath, limit); err == nil {
		_, _, e := ActiveFor(root, id, limit)
		return e
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	b, err := DefaultFor(id)
	if err != nil {
		return err
	}
	digest, err := Put(root, b)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(Selection{Digest: digest, Reason: "initial built-in public skill; personal policy not yet reviewed"})
	return files.Write(root, activePath, data, false)
}

func Load(root, digest string, limit int64) (Bundle, error) {
	return LoadFor(root, mail.Workgroup, digest, limit)
}

func LoadFor(root, id, digest string, limit int64) (Bundle, error) {
	var b Bundle
	p, err := bundlePathFor(id, digest)
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
	if b.Version == 1 {
		if id != mail.Workgroup {
			return b, errors.New("legacy bundle belongs to mail-review")
		}
	} else if b.Version == BundleVersion && b.Workgroup != id {
		return b, errors.New("workgroup bundle does not match requested workgroup")
	}
	if err = b.Validate(); err != nil {
		return b, err
	}
	return b, nil
}

func Active(root string, limit int64) (Bundle, string, error) {
	return ActiveFor(root, mail.Workgroup, limit)
}

func ActiveFor(root, id string, limit int64) (Bundle, string, error) {
	activePath, err := ActivePathFor(id)
	if err != nil {
		return Bundle{}, "", err
	}
	var selected Selection
	data, err := files.Read(root, activePath, limit)
	if err != nil {
		return Bundle{}, "", err
	}
	if err = mail.Decode(data, &selected); err != nil {
		return Bundle{}, "", err
	}
	b, err := LoadFor(root, id, selected.Digest, limit)
	return b, selected.Digest, err
}

func (b Bundle) Validate() error {
	if !json.Valid(b.Schema) {
		return errors.New("invalid workgroup schema")
	}
	if _, err := mail.CompileSchema(b.Schema); err != nil {
		return err
	}
	switch b.Version {
	case 1:
		if !mail.Nonempty(b.Guide) || b.Workgroup != "" || b.Skill != nil {
			return errors.New("invalid legacy workgroup bundle")
		}
		return nil
	case BundleVersion:
		if !allowedWorkgroup(b.Workgroup) {
			return errors.New("unsupported workgroup")
		}
		_, err := b.SelectedSkill()
		return err
	default:
		return errors.New("unsupported workgroup bundle version")
	}
}

func allowedWorkgroup(id string) bool {
	return id == mail.Workgroup || id == "jira-report"
}

func readEmbeddedSkill(id string) (Skill, error) {
	data, err := workgroups.Assets.ReadFile(filepath.ToSlash(filepath.Join(id, "SKILL.md")))
	if err != nil {
		return Skill{}, err
	}
	return parseSkill(data)
}

func legacyMailSkill(guide string) (Skill, error) {
	base, err := readEmbeddedSkill(mail.Workgroup)
	if err != nil {
		return Skill{}, err
	}
	base.Markdown = skillDocument(base.Name, base.Description, guide)
	return base, base.Validate()
}

func skillDocument(name, description, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s", name, description, body)
}
