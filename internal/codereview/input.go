// Package codereview implements immutable, explicitly selected Git-file review.
package codereview

import (
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
)

const Workgroup = "code-review"
const Version = 1

type Source struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Diff         string `json:"diff"`
	BeforeDigest string `json:"before_digest"`
	AfterDigest  string `json:"after_digest"`
}
type Input struct {
	Version      int      `json:"version"`
	Repository   string   `json:"repository"`
	BaseRevision string   `json:"base_revision"`
	HeadRevision string   `json:"head_revision"`
	CapturedAt   string   `json:"captured_at"`
	Synthetic    bool     `json:"synthetic"`
	Sources      []Source `json:"sources"`
}

func validPath(p string) bool {
	return p != "" && p != "." && path.Clean(p) == p && !path.IsAbs(p) && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00\r\n") && p != ".."
}

func revision(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func Parse(data []byte, limits config.Limits) (Input, error) {
	var in Input
	if err := mail.Decode(data, &in); err != nil {
		return in, err
	}
	if in.Version != Version || !mail.Nonempty(in.Repository) || !revision(in.BaseRevision) || !revision(in.HeadRevision) || len(in.Sources) == 0 || len(in.Sources) > limits.MaxMessages || int64(len(data)) > limits.MaxArtifactBytes {
		return in, errors.New("invalid code-review snapshot or configured source budget exceeded")
	}
	if _, err := time.Parse(time.RFC3339Nano, in.CapturedAt); err != nil {
		return in, err
	}
	seen := map[string]bool{}
	for _, source := range in.Sources {
		if !validPath(source.Path) || source.ID != files.Digest([]byte(source.Path)) || seen[source.ID] {
			return in, errors.New("invalid or duplicate code source path/identity")
		}
		seen[source.ID] = true
		if !utf8.ValidString(source.Before+source.After+source.Diff) || strings.ContainsRune(source.Before+source.After+source.Diff, '\x00') || int64(len(source.Before)+len(source.After)+len(source.Diff)) > limits.MaxSourceBytes {
			return in, errors.New("code source must be bounded UTF-8 text")
		}
		if source.BeforeDigest != files.Digest([]byte(source.Before)) || source.AfterDigest != files.Digest([]byte(source.After)) {
			return in, errors.New("code source content digest differs")
		}
	}
	return in, nil
}

func Index(in Input) ([]byte, error) {
	type entry struct {
		ID           string `json:"id"`
		Path         string `json:"path"`
		BeforeDigest string `json:"before_digest"`
		AfterDigest  string `json:"after_digest"`
	}
	entries := []entry{}
	for _, s := range in.Sources {
		entries = append(entries, entry{s.ID, s.Path, s.BeforeDigest, s.AfterDigest})
	}
	return json.MarshalIndent(map[string]any{"repository": in.Repository, "base_revision": in.BaseRevision, "head_revision": in.HeadRevision, "captured_at": in.CapturedAt, "synthetic": in.Synthetic, "sources": entries, "scope": "selected committed files only; working tree and other files excluded"}, "", "  ")
}

func Authorize(in Input, route config.Executor) error {
	if !in.Synthetic && !route.LiveCodeApproved {
		return errors.New("code disclosure is disabled for this AI route; review the selected files and approve the route before execution")
	}
	return nil
}
