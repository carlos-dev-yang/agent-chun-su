package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"chunsu/internal/files"
)

const (
	Version                  = 1
	AppName                  = "chunsu"
	HomeEnv                  = "CHUNSU_HOME"
	FileName                 = "config.json"
	DefaultTimeoutSeconds    = 300
	DefaultLockWaitSeconds   = 5
	DefaultMaxAttempts       = 2
	DefaultMaxArtifactBytes  = 8 << 20
	DefaultMaxSourceBytes    = 128 << 10
	DefaultMaxMessages       = 50
	DefaultRetryDelaySeconds = 30
	DefaultPollSeconds       = 2
	DefaultMaxToolCalls      = 200
	DefaultMaxEvidenceBytes  = 32 << 20
	DefaultMaxBackupBytes    = 1 << 30
	MaxDurationSeconds       = math.MaxInt64 / int64(time.Second)
)

type Limits struct {
	TimeoutSeconds    int   `json:"timeout_seconds"`
	LockWaitSeconds   int   `json:"lock_wait_seconds"`
	MaxAttempts       int   `json:"max_attempts"`
	MaxArtifactBytes  int64 `json:"max_artifact_bytes"`
	MaxSourceBytes    int64 `json:"max_source_bytes"`
	MaxMessages       int   `json:"max_messages"`
	RetryDelaySeconds int   `json:"retry_delay_seconds"`
	PollSeconds       int   `json:"poll_seconds"`
	MaxToolCalls      int   `json:"max_tool_calls"`
	MaxEvidenceBytes  int64 `json:"max_evidence_bytes"`
	MaxBackupBytes    int64 `json:"max_backup_bytes"`
}

type Executor struct {
	Kind             string `json:"kind"`
	Path             string `json:"path"`
	Model            string `json:"model,omitempty"`
	Environment      string `json:"environment,omitempty"`
	LiveMailApproved bool   `json:"live_mail_approved"`
	LiveCodeApproved bool   `json:"live_code_approved,omitempty"`
	// LiveJiraApproved is deliberately separate from mail approval. A successful
	// mail boundary check establishes nothing about Jira source disclosure.
	LiveJiraApproved     bool   `json:"live_jira_approved"`
	LiveJiraPolicyDigest string `json:"live_jira_policy_digest,omitempty"`
	// LiveJiraValidationJobID names the completed synthetic Jira attempt that
	// proved the active Skill and scoped tool boundary for this executor.
	LiveJiraValidationJobID string `json:"live_jira_validation_job_id,omitempty"`
}

const RoleTask = "task"
const RoleReception = "reception"
const RoleReview = "review"

type Routes struct {
	Reception *Executor `json:"reception,omitempty"`
	Review    *Executor `json:"review,omitempty"`
}

func (e *Executor) RevokeDisclosure() {
	e.LiveMailApproved = false
	e.LiveCodeApproved = false
	e.LiveJiraApproved = false
	e.LiveJiraValidationJobID = ""
}

func (c *Config) RevokeDisclosures() {
	c.Executor.RevokeDisclosure()
	if c.Routes.Reception != nil {
		c.Routes.Reception.RevokeDisclosure()
	}
	if c.Routes.Review != nil {
		c.Routes.Review.RevokeDisclosure()
	}
}

type Config struct {
	Version  int      `json:"version"`
	Limits   Limits   `json:"limits"`
	Executor Executor `json:"executor"`
	Routes   Routes   `json:"routes,omitempty"`
	MailMode string   `json:"mail_mode"`
	Timezone string   `json:"timezone"`
}

func (c Config) ExecutorFor(role string) Executor {
	if role == RoleReception && c.Routes.Reception != nil {
		return *c.Routes.Reception
	}
	if role == RoleReview && c.Routes.Review != nil {
		return *c.Routes.Review
	}
	return c.Executor
}

func Defaults() Config {
	zone := time.Now().Location().String()
	if zone == "Local" {
		zone = "UTC"
	}
	return Config{Version: Version, Timezone: zone, MailMode: "changes", Limits: Limits{
		TimeoutSeconds: DefaultTimeoutSeconds, LockWaitSeconds: DefaultLockWaitSeconds,
		MaxAttempts: DefaultMaxAttempts, MaxArtifactBytes: DefaultMaxArtifactBytes,
		MaxSourceBytes: DefaultMaxSourceBytes, MaxMessages: DefaultMaxMessages,
		RetryDelaySeconds: DefaultRetryDelaySeconds, PollSeconds: DefaultPollSeconds,
		MaxToolCalls:     DefaultMaxToolCalls,
		MaxEvidenceBytes: DefaultMaxEvidenceBytes,
		MaxBackupBytes:   DefaultMaxBackupBytes,
	}}
}

func Resolve(override string) (string, error) {
	p := override
	if p == "" {
		p = os.Getenv(HomeEnv)
	}
	if p == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(base, AppName)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	userRoot, _ := os.UserHomeDir()
	if abs == string(filepath.Separator) || abs == userRoot {
		return "", errors.New("choose a dedicated application data directory")
	}
	return filepath.Clean(abs), nil
}

func Setup(root string) (bool, error) {
	if err := files.PrivateDir(root); err != nil {
		return false, err
	}
	for _, dir := range []string{"state", "runs", "workgroups", "evaluations", "proposals", "backups"} {
		if err := files.PrivateDir(filepath.Join(root, dir)); err != nil {
			return false, err
		}
	}
	p := filepath.Join(root, FileName)
	if _, err := os.Lstat(p); err == nil {
		_, err = Load(root)
		return false, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	b, err := json.MarshalIndent(Defaults(), "", "  ")
	if err != nil {
		return false, err
	}
	return true, files.Write(root, FileName, append(b, '\n'), false)
}

func Load(root string) (Config, error) {
	c := Defaults()
	if err := files.RequirePrivateDir(root); err != nil {
		return c, err
	}
	if err := files.RequirePrivateDir(filepath.Join(root, "state")); err != nil {
		return c, err
	}
	b, err := files.Read(root, FileName, DefaultMaxArtifactBytes)
	if err != nil {
		return c, fmt.Errorf("read configuration (run setup first): %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil || !json.Valid(b) {
		return c, fmt.Errorf("invalid configuration: %w", err)
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("unsupported configuration version %d", c.Version)
	}
	l := c.Limits
	if l.TimeoutSeconds <= 0 || l.LockWaitSeconds <= 0 || l.MaxAttempts <= 0 || l.MaxArtifactBytes <= 0 || l.MaxSourceBytes <= 0 || l.MaxMessages <= 0 || l.RetryDelaySeconds <= 0 || l.PollSeconds <= 0 || l.MaxToolCalls <= 0 || l.MaxEvidenceBytes <= 0 || l.MaxBackupBytes <= 0 {
		return errors.New("operational limits must be positive")
	}
	for _, seconds := range []int{l.TimeoutSeconds, l.LockWaitSeconds, l.RetryDelaySeconds, l.PollSeconds} {
		if int64(seconds) > MaxDurationSeconds {
			return errors.New("configured duration exceeds supported range")
		}
	}
	for _, size := range []int64{l.MaxArtifactBytes, l.MaxSourceBytes, l.MaxEvidenceBytes, l.MaxBackupBytes} {
		if size >= math.MaxInt64 {
			return errors.New("configured byte budget exceeds supported range")
		}
	}
	for _, selected := range []Executor{c.ExecutorFor(RoleTask), c.ExecutorFor(RoleReception), c.ExecutorFor(RoleReview)} {
		if selected.LiveJiraPolicyDigest != "" && !files.ValidDigest(selected.LiveJiraPolicyDigest) {
			return errors.New("live_jira_policy_digest must be a SHA-256 digest")
		}
	}
	if c.MailMode != "changes" && c.MailMode != "changes_and_open" {
		return errors.New("mail_mode must be changes or changes_and_open")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	return nil
}

func Save(root string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(root, FileName, append(b, '\n'), true)
}
