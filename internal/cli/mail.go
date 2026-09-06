package cli

import (
	"errors"
	"path/filepath"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/gateway"
	"chunsu/internal/mail"
	"chunsu/internal/workgroup"
	"github.com/spf13/cobra"
)

func readExternal(path string, limit int64) ([]byte, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return files.Read(filepath.Dir(p), filepath.Base(p), limit)
}
func (o *options) mailCommands() *cobra.Command {
	cmd := &cobra.Command{Use: "mail", Short: "Inspect saved mail and validate local report contracts"}
	cmd.AddCommand(&cobra.Command{Use: "inspect INPUT.json", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c := config.Defaults()
		b, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		snapshot, err := mail.ParseSnapshot(b, c.Limits)
		if err != nil {
			return err
		}
		targets := 0
		for _, m := range snapshot.Messages {
			if m.Scope == mail.Target {
				targets++
			}
		}
		return output(cmd, map[string]any{"valid": true, "synthetic": snapshot.Synthetic, "as_of": snapshot.AsOf, "timezone": snapshot.Timezone, "messages": len(snapshot.Messages), "targets": targets, "collection": snapshot.Collection})
	}})
	cmd.AddCommand(&cobra.Command{Use: "validate-report INPUT.json REPORT.json", Short: "Check structure and source references offline; this does not complete a job", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		b, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		snapshot, err := mail.ParseSnapshot(b, c.Limits)
		if err != nil {
			return err
		}
		result, err := readExternal(args[1], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		bundle, _, err := workgroup.Active(root, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		_, v, err := mail.ValidateReport(result, bundle.Schema, snapshot, c.MailMode, nil)
		if err != nil {
			_ = output(cmd, map[string]any{"valid": false, "error": err.Error()})
			return errors.New("report contract validation failed")
		}
		return output(cmd, map[string]any{"validation": v, "scope": "offline contract checks only; no execution or gateway evidence verified"})
	}})
	return cmd
}
func (o *options) tools() *cobra.Command {
	cmd := &cobra.Command{Use: "tools", Hidden: true}
	cmd.AddCommand(&cobra.Command{Use: "serve JOB_ID ATTEMPT_ID", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		return gateway.Serve(cmd.Context(), root, args[0], args[1])
	}})
	return cmd
}
