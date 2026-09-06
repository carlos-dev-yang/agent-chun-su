package cli

import (
	"fmt"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/service"
	"github.com/spf13/cobra"
)

func (o *options) service() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Prepare or explicitly manage an optional macOS user LaunchAgent"}
	for _, operation := range []string{"render", "install"} {
		var atLogin bool
		child := &cobra.Command{Use: operation, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			root, err := o.path()
			if err != nil {
				return err
			}
			var definition service.Definition
			if operation == "install" {
				_, _, close, e := o.open(cmd.Context(), true)
				if e != nil {
					return e
				}
				defer close()
				definition, err = service.Install(root, atLogin)
			} else {
				definition, err = service.Render(root, atLogin)
			}
			if err != nil {
				return err
			}
			if o.json || operation == "install" {
				return output(cmd, definition)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), definition.Body)
			return err
		}}
		child.Flags().BoolVar(&atLogin, "at-login", false, "Opt in to starting at future user login; not enabled by default")
		cmd.AddCommand(child)
	}
	for _, operation := range []string{"start", "status", "stop", "remove"} {
		cmd.AddCommand(&cobra.Command{Use: operation, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			root, err := o.path()
			if err != nil {
				return err
			}
			c, err := config.Load(root)
			if err != nil {
				return err
			}
			timeout := time.Duration(c.Limits.LockWaitSeconds) * time.Second
			if operation == "remove" {
				d, err := service.Remove(cmd.Context(), root, timeout)
				if err != nil {
					return err
				}
				return output(cmd, map[string]any{"removed": d.Path, "data_preserved": true})
			}
			result, runErr := service.Command(cmd.Context(), root, operation, timeout)
			if err = output(cmd, result); err != nil {
				return err
			}
			return runErr
		}})
	}
	return cmd
}
