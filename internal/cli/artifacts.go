package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"chunsu/internal/backup"
	"chunsu/internal/config"
	"chunsu/internal/runner"
	"github.com/spf13/cobra"
)

func (o *options) report() *cobra.Command {
	var pathOnly bool
	cmd := &cobra.Command{Use: "report JOB_ID", Short: "Read a locally available report without marking it as acknowledged", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		j, err := s.Job(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		artifacts, err := s.Artifacts(cmd.Context(), j.ID)
		if err != nil {
			return err
		}
		for i := len(artifacts) - 1; i >= 0; i-- {
			art := artifacts[i]
			if art.AttemptID == j.CurrentAttempt && art.Kind == "report_markdown" {
				b, err := s.ReadArtifact(art, c.Limits.MaxArtifactBytes)
				if err != nil {
					return err
				}
				path := filepath.Join(s.Root, art.Path)
				if o.json {
					return output(cmd, map[string]any{"artifact": art, "path": path, "job_status": j.Status, "availability": "local", "acknowledgment": "not inferred"})
				}
				if pathOnly {
					fmt.Fprintln(cmd.OutOrStdout(), path)
				} else {
					_, err = cmd.OutOrStdout().Write(b)
				}
				return err
			}
		}
		return errors.New("no report is locally available; inspect the job or use publish for a presentation failure")
	}}
	cmd.Flags().BoolVar(&pathOnly, "path", false, "Print only the verified report file path")
	return cmd
}
func (o *options) publish() *cobra.Command {
	return &cobra.Command{Use: "publish JOB_ID", Short: "Recover report presentation from preserved output without rerunning AI", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if _, err = runner.Recover(cmd.Context(), s, c); err != nil {
			return err
		}
		result, err := (&runner.Runner{Store: s, Config: c}).Publish(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return output(cmd, result)
	}}
}
func (o *options) backup() *cobra.Command {
	return &cobra.Command{Use: "backup NEW_DIRECTORY", Short: "Create a consistent database and evidence backup while the controller is idle", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if _, err = runner.Recover(cmd.Context(), s, c); err != nil {
			return err
		}
		manifest, err := backup.Create(cmd.Context(), s, c, args[0])
		if err != nil {
			return err
		}
		return output(cmd, manifest)
	}}
}
func (o *options) restore() *cobra.Command {
	return &cobra.Command{Use: "restore BACKUP_DIRECTORY NEW_DATA_DIRECTORY", Short: "Verify and restore into a separate private data root with accounts disabled", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		path, err := backup.Restore(cmd.Context(), args[0], args[1], config.DefaultMaxBackupBytes)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"root": path, "connections_enabled": false, "action": "review configuration and reconnect before resuming work"})
	}}
}
func (o *options) verifyBackup() *cobra.Command {
	return &cobra.Command{Use: "verify-backup DIRECTORY", Short: "Verify every declared backup file without connecting accounts", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := backup.Verify(cmd.Context(), args[0], config.DefaultMaxBackupBytes)
		if err != nil {
			return err
		}
		return output(cmd, manifest)
	}}
}
