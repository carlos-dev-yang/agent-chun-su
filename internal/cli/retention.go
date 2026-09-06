package cli

import (
	"errors"

	"chunsu/internal/retention"
	"chunsu/internal/runner"
	"github.com/spf13/cobra"
)

func (o *options) retention() *cobra.Command {
	cmd := &cobra.Command{Use: "retention", Short: "Review and explicitly remove one finished run's local content"}
	cmd.AddCommand(&cobra.Command{Use: "plan JOB_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if _, err = runner.Recover(cmd.Context(), s, c); err != nil {
			return err
		}
		p, err := retention.Create(cmd.Context(), s, c, args[0])
		if err != nil {
			return err
		}
		return output(cmd, p)
	}})
	cmd.AddCommand(&cobra.Command{Use: "show PLAN_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		op, p, err := retention.Read(cmd.Context(), s, c, args[0])
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"operation": op, "plan": p})
	}})
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		ops, err := retention.Pending(cmd.Context(), s)
		if err != nil {
			return err
		}
		return output(cmd, ops)
	}})
	var confirm string
	apply := &cobra.Command{Use: "apply PLAN_ID", Short: "Delete exactly the reviewed run content; repeat an interrupted operation with the same ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !retention.ValidConfirmation(args[0], confirm) {
			return errors.New("review retention show, then pass --confirm PLAN_ID to apply the irreversible logical deletion")
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if _, err = runner.Recover(cmd.Context(), s, c); err != nil {
			return err
		}
		op, err := retention.Apply(cmd.Context(), s, c, args[0])
		if err != nil {
			return err
		}
		return output(cmd, op)
	}}
	apply.Flags().StringVar(&confirm, "confirm", "", "The reviewed plan ID, acknowledging its stated deletion limits")
	cmd.AddCommand(apply)
	return cmd
}
