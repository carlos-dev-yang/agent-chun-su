package cli

import (
	"chunsu/internal/backend"
	"chunsu/internal/config"
	"github.com/spf13/cobra"
)

// controller exposes the independent backend lifecycle. It does not make the
// chat receiver an owner of controller processes or SQLite recovery.
func (o *options) controller() *cobra.Command {
	var managed bool
	cmd := &cobra.Command{Use: "controller", Short: "Manage the local controller service"}
	serve := &cobra.Command{Use: "serve", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return backend.Serve(cmd.Context(), root, c, managed)
	}}
	serve.Flags().BoolVar(&managed, "managed", false, "Run under the configured user service")
	cmd.AddCommand(serve)
	for _, operation := range []string{"start", "stop", "restart", "status"} {
		cmd.AddCommand(o.runtimeCommand("controller", operation))
	}
	return cmd
}

func (o *options) runtimeCommand(component, operation string) *cobra.Command {
	return &cobra.Command{Use: operation, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		result, runErr := backend.Command(cmd.Context(), root, c, component, operation)
		if err = output(cmd, result); err != nil {
			return err
		}
		return runErr
	}}
}
