package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/platform"
	"chunsu/internal/runner"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
	"github.com/spf13/cobra"
)

type options struct {
	root    string
	json    bool
	version string
}

func New(version string) *cobra.Command {
	o := &options{version: version}
	root := &cobra.Command{Use: "chunsu", Short: "Personal AI workflows with human-owned controls", Version: version, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&o.root, "home", "", "Application data directory (or CHUNSU_HOME)")
	root.PersistentFlags().BoolVar(&o.json, "json", false, "Print structured JSON")
	root.AddCommand(o.setup(), o.doctor(), o.configuration(), o.queue(), o.jobs(), o.show(), o.logs(), o.cancel(), o.recover())
	root.AddCommand(o.mailCommands(), o.tools())
	root.AddCommand(o.run(), o.resume(false), o.resume(true), o.worker())
	root.AddCommand(o.feedback(), o.workgroup(), o.experiment())
	root.AddCommand(o.gmail())
	root.AddCommand(o.report(), o.publish(), o.backup(), o.restore(), o.verifyBackup())
	return root
}

func (o *options) path() (string, error) { return config.Resolve(o.root) }

func (o *options) open(ctx context.Context, write bool) (*store.Store, config.Config, func(), error) {
	root, err := o.path()
	if err != nil {
		return nil, config.Config{}, nil, err
	}
	c, err := config.Load(root)
	if err != nil {
		return nil, c, nil, err
	}
	var lock *platform.Lock
	if write {
		lock, err = platform.Acquire(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return nil, c, nil, err
		}
	}
	var s *store.Store
	if write {
		s, err = store.Open(ctx, root, c, false)
	} else {
		s, err = store.OpenReadOnly(ctx, root)
	}
	if err != nil {
		if lock != nil {
			lock.Close()
		}
		return nil, c, nil, err
	}
	return s, c, func() {
		s.Close()
		if lock != nil {
			lock.Close()
		}
	}, nil
}

func output(cmd *cobra.Command, v any) error {
	e := json.NewEncoder(cmd.OutOrStdout())
	e.SetIndent("", "  ")
	return e.Encode(v)
}

func (o *options) setup() *cobra.Command {
	cmd := &cobra.Command{Use: "setup", Short: "Initialize private local settings and SQLite without replacing existing data"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		created, err := config.Setup(root)
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		lock, err := platform.Acquire(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		s, err := store.Open(cmd.Context(), root, c, true)
		if err != nil {
			return err
		}
		defer s.Close()
		if err = workgroup.Install(root, c.Limits.MaxArtifactBytes); err != nil {
			return err
		}
		if o.json {
			return output(cmd, map[string]any{"root": root, "created": created, "schema_version": store.SchemaVersion, "executor_configured": c.Executor.Path != ""})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Ready:", root)
		fmt.Fprintln(cmd.OutOrStdout(), "Settings and data preserved. Run doctor to inspect readiness.")
		return nil
	}
	return cmd
}

func (o *options) doctor() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Inspect local readiness without connecting accounts", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		checks := map[string]any{"root": root, "version": o.version, "go_runtime": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}
		c, err := config.Load(root)
		if err != nil {
			checks["configuration_error"] = err.Error()
			_ = output(cmd, checks)
			return errors.New("local configuration is not ready")
		}
		checks["configuration"] = "valid"
		s, err := store.OpenReadOnly(cmd.Context(), root)
		if err != nil {
			checks["database_error"] = err.Error()
			_ = output(cmd, checks)
			return errors.New("database is not ready")
		}
		defer s.Close()
		var engine string
		if err = s.DB.QueryRowContext(cmd.Context(), "SELECT sqlite_version()").Scan(&engine); err != nil {
			return err
		}
		checks["sqlite_engine"] = engine
		checks["schema_version"] = store.SchemaVersion
		checks["executor"] = "not configured"
		if c.Executor.Path != "" {
			p, e := exec.LookPath(c.Executor.Path)
			if e != nil {
				checks["executor"] = "executable unavailable"
			} else {
				checks["executor"] = p
			}
		}
		checks["account_requirement"] = "none for saved-input setup"
		return output(cmd, checks)
	}}
}

func (o *options) configuration() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect or change non-secret settings"}
	cmd.AddCommand(&cobra.Command{Use: "show", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return output(cmd, c)
	}})
	cmd.AddCommand(&cobra.Command{Use: "set KEY VALUE", Args: cobra.ExactArgs(2), Short: "Set timezone, mail_mode, executor fields, or named limits", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		lock, err := platform.Acquire(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		// Reload while holding ownership so concurrent configuration updates are not lost.
		c, err = config.Load(root)
		if err != nil {
			return err
		}
		switch args[0] {
		case "timezone":
			c.Timezone = args[1]
		case "mail_mode":
			c.MailMode = args[1]
		case "executor.kind":
			c.Executor.Kind = args[1]
		case "executor.path":
			c.Executor.Path = args[1]
		case "executor.model":
			c.Executor.Model = args[1]
		default:
			v, e := strconv.Atoi(args[1])
			if e != nil {
				return errors.New("limit must be an integer")
			}
			switch args[0] {
			case "limits.timeout_seconds":
				c.Limits.TimeoutSeconds = v
			case "limits.lock_wait_seconds":
				c.Limits.LockWaitSeconds = v
			case "limits.max_attempts":
				c.Limits.MaxAttempts = v
			case "limits.max_artifact_bytes":
				c.Limits.MaxArtifactBytes = int64(v)
			case "limits.max_source_bytes":
				c.Limits.MaxSourceBytes = int64(v)
			case "limits.max_messages":
				c.Limits.MaxMessages = v
			case "limits.retry_delay_seconds":
				c.Limits.RetryDelaySeconds = v
			case "limits.poll_seconds":
				c.Limits.PollSeconds = v
			case "limits.max_tool_calls":
				c.Limits.MaxToolCalls = v
			case "limits.max_evidence_bytes":
				c.Limits.MaxEvidenceBytes = int64(v)
			case "limits.max_backup_bytes":
				c.Limits.MaxBackupBytes = int64(v)
			default:
				return errors.New("unknown configuration key")
			}
		}
		if err = config.Save(root, c); err != nil {
			return err
		}
		return output(cmd, map[string]string{"updated": args[0]})
	}})
	return cmd
}

func (o *options) queue() *cobra.Command {
	return &cobra.Command{Use: "queue INPUT.json", Short: "Snapshot a saved JSON input and queue a mail-review job", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		cfg, err := config.Load(root)
		if err != nil {
			return err
		}
		input, err := readExternal(args[0], cfg.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		if handled, err := o.managed(cmd, control.Request{Operation: "queue", Input: input}); handled || err != nil {
			return err
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		p, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		b, err := files.Read(filepath.Dir(p), filepath.Base(p), c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		if _, err = mail.ParseSnapshot(b, c.Limits); err != nil {
			return fmt.Errorf("mail snapshot: %w", err)
		}
		j, err := s.Submit(cmd.Context(), "mail-review", b, map[string]any{"origin": "saved", "source_name": filepath.Base(p)}, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		if o.json {
			return output(cmd, j)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Queued", j.ID)
		return nil
	}}
}

func (o *options) jobs() *cobra.Command {
	return &cobra.Command{Use: "jobs", Short: "List persisted jobs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		jobs, err := s.Jobs(cmd.Context())
		if err != nil {
			return err
		}
		if o.json {
			return output(cmd, jobs)
		}
		for _, j := range jobs {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  %-14s  %s\n", j.ID, j.Status, j.Workgroup)
		}
		return nil
	}}
}

func (o *options) show() *cobra.Command {
	return &cobra.Command{Use: "show JOB_ID", Short: "Inspect a job, attempts, and artifact references", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		j, err := s.Job(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		a, err := s.Attempts(cmd.Context(), j.ID)
		if err != nil {
			return err
		}
		art, err := s.Artifacts(cmd.Context(), j.ID)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"job": j, "attempts": a, "artifacts": art})
	}}
}

func (o *options) logs() *cobra.Command {
	return &cobra.Command{Use: "logs JOB_ID", Short: "Read observable events; private reasoning is not recorded", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		if _, err = s.Job(cmd.Context(), args[0]); err != nil {
			return err
		}
		events, err := s.Events(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return output(cmd, events)
	}}
}

func (o *options) cancel() *cobra.Command {
	return &cobra.Command{Use: "cancel JOB_ID", Short: "Cancel a job and terminate its active executor if running", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if handled, err := o.managed(cmd, control.Request{Operation: "cancel", JobID: args[0]}); handled || err != nil {
			return err
		}
		s, _, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if err = s.Cancel(cmd.Context(), args[0]); err != nil {
			return err
		}
		return output(cmd, map[string]string{"job_id": args[0], "status": store.Cancelled})
	}}
}

func (o *options) recover() *cobra.Command {
	return &cobra.Command{Use: "recover", Short: "Identify interrupted work after obtaining controller ownership", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		n, err := runner.Recover(cmd.Context(), s, c)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"interrupted": n, "action": "review before retry"})
	}}
}
