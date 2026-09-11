package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/secrets"
)

const (
	CloudProvider       = "jira-cloud"
	LiveReaderKind      = "jira-cloud-v1"
	KeychainSecretStore = secrets.KeychainStore
)

// SecretRef is a user-owned Keychain binding. It never contains a token.
type SecretRef struct {
	Store   string `json:"store"`
	Service string `json:"service"`
	Account string `json:"account"`
}

type LivePolicy struct {
	Selection    string `json:"selection"`
	WindowDays   int    `json:"window_days"`
	ContentScope string `json:"content_scope"`
	MaxIssues    int    `json:"max_issues"`
	MaxPages     int    `json:"max_pages"`
}

// Profile is local connection configuration. The normalized acquisition pins
// its non-secret policy digest; executor packages never receive this profile.
type Profile struct {
	Version          int        `json:"version"`
	ID               string     `json:"id"`
	Provider         string     `json:"provider"`
	SiteHost         string     `json:"site_host"`
	CloudID          string     `json:"cloud_id"`
	ExpectedAccount  string     `json:"expected_account"`
	SubjectAccountID string     `json:"subject_account_id,omitempty"`
	ProjectKey       string     `json:"project_key"`
	BoardID          string     `json:"board_id"`
	TodoStatusID     string     `json:"todo_status_id"`
	TodoStatusName   string     `json:"todo_status_name"`
	DueField         string     `json:"due_field"`
	StartField       string     `json:"start_field"`
	Timezone         string     `json:"timezone"`
	Secret           SecretRef  `json:"secret"`
	Policy           LivePolicy `json:"policy"`
	Active           bool       `json:"active"`
	VerifiedAt       string     `json:"verified_at,omitempty"`
}

// ReportPolicy is the exact report-runtime policy derived from a connection.
// Site/cloud identity stays a separate input/profile comparison; the report
// policy digest is deliberately identical to the queued ReportInput policy.
func (p Profile) ReportPolicy() ReportPolicy {
	return ReportPolicy{Version: ReportPolicyVersion, ConnectionID: p.ID, ProjectKeys: []string{p.ProjectKey}, BoardIDs: []string{p.BoardID}, SubjectAccountID: p.SubjectAccountID, SubjectLabel: p.ExpectedAccount, Selection: p.Policy.Selection, TodoStatusID: p.TodoStatusID, DueField: p.DueField, StartField: p.StartField, Timezone: p.Timezone, WindowDays: p.Policy.WindowDays, ContentScope: p.Policy.ContentScope}
}
func (p Profile) ReportPolicyDigest() string { return ReportPolicyDigest(p.ReportPolicy()) }

func ProfilePath(id string) (string, error) {
	if !files.ValidID(id) {
		return "", errors.New("invalid Jira connection ID")
	}
	return filepath.ToSlash(filepath.Join("state", "connections", CloudProvider, id+".json")), nil
}

func plainText(value string, max int) bool {
	return value != "" && len(value) <= max && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}
func decimal(value string) bool {
	if !plainText(value, 64) || value == "0" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func fieldID(value string) bool {
	if !plainText(value, 128) {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}
func projectKey(value string) bool {
	if !plainText(value, 64) {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}
func cloudID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func (r SecretRef) Validate(required bool) error {
	if !required && r.Store == "" && r.Service == "" && r.Account == "" {
		return nil
	}
	if (r.Store != KeychainSecretStore && r.Store != secrets.HelperStore) || !plainText(r.Service, 256) {
		return errors.New("Jira secret must name a supported credential store and bounded service reference")
	}
	a, err := mail.ParseAddress(r.Account)
	if err != nil || a.Address != r.Account {
		return errors.New("Jira Keychain account must be a plain email address")
	}
	return nil
}
func (p LivePolicy) Validate(c config.Config) error {
	if p.Selection != GroupedSelection {
		return errors.New("unsupported Jira selection policy")
	}
	if p.WindowDays != ReportWindowDays {
		return errors.New("Jira report window must be 14 calendar days")
	}
	if p.ContentScope != "metadata_only" && p.ContentScope != "metadata_and_description" {
		return errors.New("unsupported Jira content scope")
	}
	if p.MaxIssues <= 0 || p.MaxIssues > c.Limits.MaxMessages || p.MaxPages <= 0 || p.MaxPages > c.Limits.MaxMessages {
		return errors.New("Jira live issue and page limits must be positive and host-bounded")
	}
	return nil
}
func (p Profile) Validate(c config.Config) error {
	if p.Version != Version || !files.ValidID(p.ID) || p.Provider != CloudProvider || !cloudID(p.CloudID) || !projectKey(p.ProjectKey) || !decimal(p.BoardID) || !decimal(p.TodoStatusID) || !plainText(p.TodoStatusName, 256) || p.DueField != "duedate" || !strings.HasPrefix(p.StartField, "customfield_") || !fieldID(p.StartField) {
		return errors.New("invalid Jira Cloud profile binding")
	}
	u, err := url.Parse("https://" + p.SiteHost)
	if err != nil || u.Scheme != "https" || u.Host != p.SiteHost || u.Hostname() != p.SiteHost || u.Port() != "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || !strings.Contains(p.SiteHost, ".") {
		return errors.New("Jira site host must be an exact HTTPS host without path or port")
	}
	a, err := mail.ParseAddress(p.ExpectedAccount)
	if err != nil || a.Address != p.ExpectedAccount {
		return errors.New("expected Jira account must be a plain email address")
	}
	if p.Timezone == "" || p.Timezone == "Local" {
		return errors.New("Jira profile requires a non-Local IANA timezone")
	}
	if _, err = time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("invalid Jira timezone: %w", err)
	}
	if err = p.Policy.Validate(c); err != nil {
		return err
	}
	if err = p.Secret.Validate(p.Active); err != nil {
		return err
	}
	if p.Active {
		if !plainText(p.SubjectAccountID, 256) || p.VerifiedAt == "" {
			return errors.New("active Jira profile requires verified subject identity")
		}
		if _, err = time.Parse(time.RFC3339Nano, p.VerifiedAt); err != nil {
			return errors.New("invalid Jira verification timestamp")
		}
	}
	return nil
}

// SaveProfile persists a validated connection profile. It loads the current
// host limits itself so backup/import paths cannot bypass profile validation.
func SaveProfile(root string, p Profile, replace bool) error {
	c, err := config.Load(root)
	if err != nil {
		return err
	}
	return saveProfile(root, p, c, replace)
}

func saveProfile(root string, p Profile, c config.Config, replace bool) error {
	if err := p.Validate(c); err != nil {
		return err
	}
	path, err := ProfilePath(p.ID)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(root, path, append(b, '\n'), replace)
}
func ReadProfile(root, id string, c config.Config) (Profile, error) {
	var p Profile
	path, err := ProfilePath(id)
	if err != nil {
		return p, err
	}
	b, err := files.Read(root, path, c.Limits.MaxArtifactBytes)
	if err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil || !json.Valid(b) {
		return p, errors.New("invalid Jira profile")
	}
	if p.ID != id {
		return p, errors.New("Jira profile ID mismatch")
	}
	return p, p.Validate(c)
}
func LoadProfile(root, id string, c config.Config) (Profile, error) {
	p, err := ReadProfile(root, id, c)
	if err != nil {
		return p, err
	}
	if !p.Active {
		return p, errors.New("Jira connection is disabled or has not been verified")
	}
	return p, nil
}

// Connect validates the external Keychain reference, then verifies the fixed
// identity, board-project binding and configured fields with GET-only requests.
func Connect(ctx context.Context, p Profile, c config.Config) (Profile, error) {
	if err := p.Validate(c); err != nil {
		return p, err
	}
	if err := p.Secret.Validate(true); err != nil {
		return p, err
	}
	k, err := secrets.OpenFor(p.Secret.Store)
	if err != nil {
		return p, err
	}
	token, err := k.GetExternal(ctx, p.Secret.Service, p.Secret.Account)
	if err != nil {
		return p, err
	}
	client, err := NewLiveClient(p, token, c.Limits)
	if err != nil {
		return p, err
	}
	me, err := client.Myself(ctx)
	if err != nil {
		return p, err
	}
	if !strings.EqualFold(me.EmailAddress, p.ExpectedAccount) || !plainText(me.AccountID, 256) {
		return p, errors.New("authenticated Jira identity differs from the selected account")
	}
	board, err := client.Board(ctx, p.BoardID, p.TodoStatusID, p.TodoStatusName)
	if err != nil {
		return p, err
	}
	if board.ProjectKey != p.ProjectKey {
		return p, errors.New("verified Jira board does not match the selected project")
	}
	if err = client.Fields(ctx, p.DueField, p.StartField); err != nil {
		return p, err
	}
	p.SubjectAccountID = me.AccountID
	p.Active = true
	p.VerifiedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return p, p.Validate(c)
}
func Check(ctx context.Context, p Profile, c config.Config) (Profile, error) {
	return Connect(ctx, p, c)
}
func Disconnect(root string, p Profile, c config.Config) error {
	p.Active = false
	p.SubjectAccountID = ""
	p.VerifiedAt = ""
	p.Secret = SecretRef{}
	return saveProfile(root, p, c, true)
}
