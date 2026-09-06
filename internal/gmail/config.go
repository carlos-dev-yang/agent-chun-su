package gmail

import (
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/files"
	mailmodel "chunsu/internal/mail"
)

const Version = 1
const ReadOnlyScope = "https://www.googleapis.com/auth/gmail.readonly"
const ProviderMaxPage = 500
const DefaultBatchSize = 20
const DefaultHTTPTimeoutSeconds = 30
const DefaultAuthTimeoutSeconds = 300
const DefaultHTTPAttempts = 3
const DefaultMaxRetrySeconds = 60
const DefaultHistoryDays = 30
const DefaultMaxMIMEDepth = 20
const DefaultMaxMIMEParts = 200
const LoopbackHost = "127.0.0.1"

type Endpoints struct {
	Authorization string `json:"authorization"`
	Token         string `json:"token"`
	API           string `json:"api"`
	Revoke        string `json:"revoke"`
}

func DefaultEndpoints() Endpoints {
	return Endpoints{Authorization: "https://accounts.google.com/o/oauth2/v2/auth", Token: "https://oauth2.googleapis.com/token", API: "https://gmail.googleapis.com/gmail/v1", Revoke: "https://oauth2.googleapis.com/revoke"}
}
func (e Endpoints) Validate() error {
	defaults := DefaultEndpoints()
	for _, pair := range [][2]string{{e.Authorization, defaults.Authorization}, {e.Token, defaults.Token}, {e.API, defaults.API}, {e.Revoke, defaults.Revoke}} {
		u, err := url.Parse(pair[0])
		expected, _ := url.Parse(pair[1])
		if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host != expected.Host || u.Path != expected.Path {
			return errors.New("Gmail endpoints must match the supported HTTPS provider routes")
		}
	}
	return nil
}

type Policy struct {
	Query         string `json:"query"`
	BatchSize     int    `json:"batch_size"`
	ThreadHistory bool   `json:"thread_history"`
	HistoryDays   int    `json:"history_days"`
	Retention     string `json:"retention"`
	Timezone      string `json:"timezone"`
}
type Connection struct {
	Version           int       `json:"version"`
	ID                string    `json:"id"`
	Account           string    `json:"account"`
	ClientID          string    `json:"client_id"`
	ClientSecretRef   string    `json:"client_secret_ref"`
	RefreshRef        string    `json:"refresh_ref"`
	Scopes            []string  `json:"scopes"`
	Policy            Policy    `json:"policy"`
	Endpoints         Endpoints `json:"endpoints"`
	Enabled           bool      `json:"enabled"`
	ConnectedAt       string    `json:"connected_at"`
	RetiredSecretRefs []string  `json:"retired_secret_refs,omitempty"`
}

func (p Policy) Validate(c config.Config) error {
	if !mailmodel.Nonempty(p.Query) || p.BatchSize <= 0 || p.BatchSize > c.Limits.MaxMessages || p.BatchSize > ProviderMaxPage || p.HistoryDays <= 0 || p.Retention != "manual" {
		return errors.New("choose a nonempty query, bounded batch size, positive history window and manual retention")
	}
	cfg := c
	cfg.Timezone = p.Timezone
	return cfg.Validate()
}
func ConnectionPath(id string) (string, error) {
	if !files.ValidID(id) {
		return "", errors.New("invalid connection ID")
	}
	return filepath.Join("state", "connections", id+".json"), nil
}
func SaveConnection(root string, connection Connection, replace bool) error {
	p, err := ConnectionPath(connection.ID)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(connection, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(root, p, b, replace)
}
func LoadConnection(root, id string, c config.Config) (Connection, error) {
	connection, err := ReadConnection(root, id, c)
	if err != nil {
		return connection, err
	}
	if !connection.Enabled {
		return connection, errors.New("connection is disabled")
	}
	return connection, nil
}

func ReadConnection(root, id string, c config.Config) (Connection, error) {
	var connection Connection
	p, err := ConnectionPath(id)
	if err != nil {
		return connection, err
	}
	b, err := files.Read(root, p, c.Limits.MaxArtifactBytes)
	if err != nil {
		return connection, err
	}
	if err = mailmodel.Decode(b, &connection); err != nil {
		return connection, err
	}
	if connection.Version != Version || connection.ID != id || !files.ValidID(connection.RefreshRef) || !files.ValidID(connection.ClientSecretRef) || connection.ClientID == "" {
		return connection, errors.New("invalid connection binding")
	}
	if err = CheckScopes(connection.Scopes); err != nil {
		return connection, err
	}
	if err = connection.Policy.Validate(c); err != nil {
		return connection, err
	}
	if err = connection.Endpoints.Validate(); err != nil {
		return connection, err
	}
	return connection, nil
}
func CheckScopes(scopes []string) error {
	if len(scopes) != 1 || scopes[0] != ReadOnlyScope {
		return errors.New("grant must contain only gmail.readonly; broader or missing scopes are rejected")
	}
	return nil
}
func CheckAccount(expected, actual string) error {
	a, err := mail.ParseAddress(expected)
	if err != nil || a.Address != expected {
		return errors.New("expected account must be a plain email address")
	}
	if !strings.EqualFold(expected, actual) {
		return errors.New("authenticated Gmail account differs from the selected account")
	}
	return nil
}
