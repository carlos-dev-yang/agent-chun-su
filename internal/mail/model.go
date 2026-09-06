package mail

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

const Version = 1
const Workgroup = "mail-review"
const Target = "target"
const Reference = "reference"
const RemainingPagesGap = "additional mailbox pages remain; continue the acquisition explicitly"

type Attachment struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Status string `json:"status"`
}
type Message struct {
	ID            string       `json:"id"`
	ThreadID      string       `json:"thread_id"`
	ReceivedAt    string       `json:"received_at"`
	Scope         string       `json:"scope"`
	Channel       string       `json:"channel"`
	From          string       `json:"from"`
	Subject       string       `json:"subject"`
	Body          string       `json:"body"`
	ContentStatus string       `json:"content_status"`
	Attachments   []Attachment `json:"attachments"`
}
type Collection struct {
	Status string   `json:"status"`
	Errors []string `json:"errors"`
	Scope  string   `json:"scope,omitempty"`
}

type Origin struct {
	Provider      string `json:"provider"`
	ConnectionID  string `json:"connection_id"`
	AcquisitionID string `json:"acquisition_id"`
	PolicyDigest  string `json:"policy_digest"`
}
type Interpretation struct {
	AsOf      string   `json:"as_of"`
	SourceIDs []string `json:"source_ids"`
	Text      string   `json:"text"`
	Kind      string   `json:"kind"`
}
type Snapshot struct {
	Version              int              `json:"version"`
	Synthetic            bool             `json:"synthetic"`
	AsOf                 string           `json:"as_of"`
	Timezone             string           `json:"timezone"`
	Collection           Collection       `json:"collection"`
	Messages             []Message        `json:"messages"`
	PriorInterpretations []Interpretation `json:"prior_interpretations"`
	Origin               *Origin          `json:"origin,omitempty"`
}
type Schedule struct {
	Status string `json:"status"`
	When   string `json:"when"`
	Impact string `json:"impact"`
}
type Completion struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}
type Item struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Categories       []string   `json:"categories"`
	Reason           string     `json:"reason"`
	Importance       string     `json:"importance"`
	ImportanceReason string     `json:"importance_reason"`
	RequestedAction  string     `json:"requested_action"`
	Sources          []string   `json:"sources"`
	History          string     `json:"history"`
	Schedule         Schedule   `json:"schedule"`
	Completion       Completion `json:"completion"`
	Uncertainties    []string   `json:"uncertainties"`
}
type Disposition struct {
	SourceID    string   `json:"source_id"`
	Disposition string   `json:"disposition"`
	ItemIDs     []string `json:"item_ids"`
	Reason      string   `json:"reason"`
}
type Report struct {
	Version      int           `json:"version"`
	AsOf         string        `json:"as_of"`
	Timezone     string        `json:"timezone"`
	Summary      string        `json:"summary"`
	Items        []Item        `json:"items"`
	Dispositions []Disposition `json:"dispositions"`
	Limitations  []string      `json:"limitations"`
	Questions    []string      `json:"questions"`
}

func Decode(data []byte, target any) error {
	if !json.Valid(data) {
		return errors.New("invalid JSON document")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func ParseSnapshot(data []byte, limits config.Limits) (Snapshot, error) {
	var s Snapshot
	if int64(len(data)) > limits.MaxArtifactBytes {
		return s, errors.New("snapshot exceeds artifact limit")
	}
	if err := Decode(data, &s); err != nil {
		return s, err
	}
	if s.Version != Version {
		return s, errors.New("unsupported mail snapshot version")
	}
	asOf, err := time.Parse(time.RFC3339, s.AsOf)
	if err != nil {
		return s, errors.New("as_of must be RFC3339")
	}
	if _, err = time.LoadLocation(s.Timezone); err != nil {
		return s, errors.New("timezone must identify an IANA location")
	}
	if !oneOf(s.Collection.Status, "complete", "partial", "failed") {
		return s, errors.New("invalid collection status")
	}
	if s.Collection.Status == "complete" && len(s.Collection.Errors) > 0 {
		return s, errors.New("complete collection cannot hide acquisition errors")
	}
	if s.Collection.Status != "complete" && len(s.Collection.Errors) == 0 {
		return s, errors.New("incomplete collection must describe the acquisition gap")
	}
	if s.Messages == nil || s.PriorInterpretations == nil || s.Collection.Errors == nil {
		return s, errors.New("snapshot arrays must be present, even when empty")
	}
	if len(s.Messages) > limits.MaxMessages {
		return s, errors.New("snapshot exceeds message limit")
	}
	seen := map[string]bool{}
	if s.Origin != nil {
		if s.Origin.Provider != "gmail" || !files.ValidID(s.Origin.ConnectionID) || !files.ValidID(s.Origin.AcquisitionID) || !files.ValidDigest(s.Origin.PolicyDigest) {
			return s, errors.New("invalid live-source provenance")
		}
	}
	for _, m := range s.Messages {
		if s.Origin != nil && (!strings.HasPrefix(m.ID, s.Origin.ConnectionID+":") || !strings.HasPrefix(m.ThreadID, s.Origin.ConnectionID+":")) {
			return s, errors.New("source identity does not belong to the pinned connection")
		}
		if m.ID == "" || m.ThreadID == "" || seen[m.ID] {
			return s, errors.New("message and thread identities must be nonempty; messages must be unique")
		}
		seen[m.ID] = true
		t, e := time.Parse(time.RFC3339, m.ReceivedAt)
		unknownTime := m.ReceivedAt == "" && m.ContentStatus == "unavailable"
		if !unknownTime && (e != nil || t.After(asOf)) {
			return s, fmt.Errorf("message %s is outside the as-of boundary or has invalid time", m.ID)
		}
		if !oneOf(m.Scope, Target, Reference) || !oneOf(m.ContentStatus, "complete", "truncated", "unavailable") {
			return s, fmt.Errorf("invalid source declaration for %s", m.ID)
		}
		if int64(len(m.Body)) > limits.MaxSourceBytes {
			return s, fmt.Errorf("source %s exceeds the content limit", m.ID)
		}
		if m.Attachments == nil {
			return s, errors.New("attachment arrays must be present")
		}
		for _, a := range m.Attachments {
			if a.ID == "" || !oneOf(a.Status, "unsupported", "unavailable", "omitted") {
				return s, errors.New("v1 records attachment limitations; it does not claim attachment extraction")
			}
		}
	}
	for _, p := range s.PriorInterpretations {
		t, e := time.Parse(time.RFC3339, p.AsOf)
		if e != nil || t.After(asOf) {
			return s, errors.New("prior interpretation leaks future data")
		}
		if !oneOf(p.Kind, "previous_report", "human_correction") || len(p.SourceIDs) == 0 {
			return s, errors.New("prior interpretations need type and source provenance")
		}
		for _, id := range p.SourceIDs {
			if !seen[id] {
				return s, errors.New("prior interpretation references unavailable source")
			}
		}
	}
	return s, nil
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func Contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func Nonempty(s string) bool { return strings.TrimSpace(s) != "" }
