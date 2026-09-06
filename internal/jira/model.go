package jira

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

const (
	Version            = 1
	Provider           = "jira"
	NormalizerVersion  = "jira-evidence-v1"
	CloudV3            = "cloud-v3"
	RESTV2             = "rest-v2"
	SavedReaderKind    = "saved-responses-v1"
	Complete           = "complete"
	Partial            = "partial"
	NotCollected       = "not_collected"
	Available          = "available"
	Missing            = "not_returned"
	Empty              = "empty"
	Unsupported        = "unsupported"
	Invalid            = "invalid"
	Truncated          = "truncated"
	JiraTimestamp      = "2006-01-02T15:04:05.000-0700"
	issuePointerPrefix = "/issues/"
)

type Identity struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

type Mapping struct {
	Version          int    `json:"version"`
	IdentityField    string `json:"identity_field"`
	SprintField      string `json:"sprint_field,omitempty"`
	StoryPointsField string `json:"story_points_field,omitempty"`
}

func ParseMapping(data []byte) (Mapping, error) {
	var mapping Mapping
	if err := decode(data, &mapping); err != nil {
		return mapping, err
	}
	return mapping, mapping.Validate()
}

type Policy struct {
	Version           int      `json:"version"`
	ConnectionID      string   `json:"connection_id"`
	ProjectKeys       []string `json:"project_keys"`
	Subject           Identity `json:"subject"`
	Selection         string   `json:"selection"`
	SprintIDs         []string `json:"sprint_ids"`
	Timezone          string   `json:"timezone"`
	MaxIssues         int      `json:"max_issues"`
	MaxPages          int      `json:"max_pages"`
	MaxContextEntries int      `json:"max_context_entries"`
	Mapping           Mapping  `json:"mapping"`
}

func (m Mapping) Validate() error {
	if m.Version != Version || (m.IdentityField != "accountId" && m.IdentityField != "key" && m.IdentityField != "name") {
		return errors.New("mapping requires version 1 and an explicit accountId, key or name identity field")
	}
	for _, field := range []string{m.SprintField, m.StoryPointsField} {
		if field != "" && (!strings.HasPrefix(field, "customfield_") || strings.Trim(field[len("customfield_"):], "0123456789") != "" || field == "customfield_") {
			return errors.New("sprint and story-point mappings require explicit customfield IDs or an empty mapping")
		}
	}
	if m.SprintField != "" && m.SprintField == m.StoryPointsField {
		return errors.New("sprint and story points cannot use the same field")
	}
	return nil
}

func (p Policy) Validate() error {
	if p.Version != Version || !files.ValidID(p.ConnectionID) || len(p.ProjectKeys) == 0 || strings.TrimSpace(p.Subject.ID) == "" {
		return errors.New("Jira policy requires version, connection identity, allowed projects and subject identity")
	}
	if p.MaxIssues <= 0 || p.MaxPages <= 0 || p.MaxContextEntries <= 0 {
		return errors.New("Jira issue, page and context budgets must be positive")
	}
	if p.Selection != "assigned" && p.Selection != "assigned_or_sprint" {
		return errors.New("selection must be assigned or assigned_or_sprint")
	}
	if p.Selection == "assigned_or_sprint" && (len(p.SprintIDs) == 0 || p.Mapping.SprintField == "") {
		return errors.New("sprint selection requires explicit sprint IDs and a sprint field mapping")
	}
	for _, list := range [][]string{p.ProjectKeys, p.SprintIDs} {
		seen := map[string]bool{}
		for _, id := range list {
			if strings.TrimSpace(id) == "" || seen[id] {
				return errors.New("scope IDs must be nonempty and unique")
			}
			seen[id] = true
		}
	}
	if p.Timezone == "" || p.Timezone == "Local" {
		return errors.New("Jira policy requires an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return errors.New("Jira policy requires a valid IANA timezone")
	}
	return p.Mapping.Validate()
}

type Reference struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}

type Provenance struct {
	Response   Reference `json:"response"`
	Pointer    string    `json:"pointer"`
	CapturedAt string    `json:"captured_at"`
}

type Text struct {
	Value  string `json:"value"`
	Format string `json:"format"`
	Status string `json:"status"`
}

type Status struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CategoryKey string `json:"category_key"`
}

type Sprint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	BoardID     string `json:"board_id,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type Estimates struct {
	OriginalSeconds  *int64   `json:"original_seconds"`
	RemainingSeconds *int64   `json:"remaining_seconds"`
	SpentSeconds     *int64   `json:"spent_seconds"`
	StoryPoints      *float64 `json:"story_points"`
}

type Relationship struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
	IssueID   string `json:"issue_id"`
	IssueKey  string `json:"issue_key"`
	Context   string `json:"context"`
}

type ContextEntry struct {
	ID        string    `json:"id"`
	Author    *Identity `json:"author,omitempty"`
	CreatedAt string    `json:"created_at"`
	Body      *Text     `json:"body,omitempty"`
	Changes   []Change  `json:"changes,omitempty"`
}

type Change struct {
	Field    string `json:"field"`
	FieldID  string `json:"field_id,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	FromText string `json:"from_text,omitempty"`
	ToText   string `json:"to_text,omitempty"`
}

type History struct {
	Status  string         `json:"status"`
	Entries []ContextEntry `json:"entries"`
}

type Issue struct {
	ConflictingVersions bool              `json:"conflicting_versions"`
	ID                  string            `json:"id"`
	Key                 string            `json:"key"`
	ProjectID           string            `json:"project_id"`
	ProjectKey          string            `json:"project_key"`
	Summary             string            `json:"summary"`
	Description         Text              `json:"description"`
	Status              Status            `json:"status"`
	Assignee            *Identity         `json:"assignee"`
	AssignedToSubject   bool              `json:"assigned_to_subject"`
	InSelectedSprint    bool              `json:"in_selected_sprint"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
	ResolvedAt          string            `json:"resolved_at"`
	DueDate             string            `json:"due_date"`
	Resolution          string            `json:"resolution"`
	Sprints             []Sprint          `json:"sprints"`
	Estimates           Estimates         `json:"estimates"`
	Relationships       []Relationship    `json:"relationships"`
	Comments            History           `json:"comments"`
	Changelog           History           `json:"changelog"`
	Availability        map[string]string `json:"availability"`
	Limitations         []string          `json:"limitations"`
	Evidence            []Provenance      `json:"evidence"`
}

type Disposition struct {
	SourceID string     `json:"source_id"`
	Outcome  string     `json:"outcome"`
	Reason   string     `json:"reason"`
	Evidence Provenance `json:"evidence"`
}

type Snapshot struct {
	AcquisitionID   string        `json:"acquisition_id"`
	Version         int           `json:"version"`
	Provider        string        `json:"provider"`
	Format          string        `json:"format"`
	Synthetic       bool          `json:"synthetic"`
	ConnectionID    string        `json:"connection_id"`
	PolicyDigest    string        `json:"policy_digest"`
	Normalizer      string        `json:"normalizer"`
	Timezone        string        `json:"timezone"`
	CapturedFrom    string        `json:"captured_from"`
	CapturedThrough string        `json:"captured_through"`
	Status          string        `json:"status"`
	Issues          []Issue       `json:"issues"`
	Dispositions    []Disposition `json:"dispositions"`
	Gaps            []string      `json:"gaps"`
}

func decode(data []byte, target any) error {
	if !json.Valid(data) {
		return errors.New("invalid JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func digest(value any) string { data, _ := json.Marshal(value); return files.Digest(data) }
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func sourceID(connection, id string) string { return connection + ":" + id }
func timestamp(value string) (string, error) {
	for _, layout := range []string{time.RFC3339Nano, JiraTimestamp} {
		if at, err := time.Parse(layout, value); err == nil {
			return at.UTC().Format(time.RFC3339Nano), nil
		}
	}
	return "", errors.New("invalid source timestamp")
}

func validateSnapshot(s Snapshot, p Policy, limits config.Limits) error {
	if !files.ValidID(s.AcquisitionID) || s.Version != Version || s.Provider != Provider || s.ConnectionID != p.ConnectionID || s.PolicyDigest != digest(p) || s.Normalizer != NormalizerVersion || s.Timezone != p.Timezone || (s.Format != CloudV3 && s.Format != RESTV2) || (s.Status != Complete && s.Status != Partial) {
		return errors.New("Jira snapshot binding mismatch")
	}
	if len(s.Issues) > p.MaxIssues {
		return errors.New("snapshot exceeds the pinned issue budget")
	}
	seen := map[string]bool{}
	for _, issue := range s.Issues {
		if seen[issue.ID] || !strings.HasPrefix(issue.ID, p.ConnectionID+":") || !contains(p.ProjectKeys, issue.ProjectKey) || len(issue.Evidence) == 0 {
			return errors.New("invalid or duplicate issue scope/provenance")
		}
		seen[issue.ID] = true
		if issue.AssignedToSubject != (issue.Assignee != nil && issue.Assignee.ID == p.Subject.ID) {
			return errors.New("assignee binding mismatch")
		}
		selected := false
		for _, sprint := range issue.Sprints {
			if contains(p.SprintIDs, sprint.ID) {
				selected = true
			}
		}
		if issue.InSelectedSprint != selected {
			return errors.New("sprint binding mismatch")
		}
		if !issue.AssignedToSubject && !(p.Selection == "assigned_or_sprint" && issue.InSelectedSprint) {
			return errors.New("issue is outside selected work")
		}
		if int64(len(issue.Description.Value)) > limits.MaxSourceBytes {
			return fmt.Errorf("normalized issue text exceeds the source budget")
		}
	}
	return nil
}
