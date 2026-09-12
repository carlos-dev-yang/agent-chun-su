package cli

import (
	"errors"

	"chunsu/internal/config"
	"chunsu/internal/errorreport"
	"github.com/spf13/cobra"
)

func (o *options) errors() *cobra.Command {
	cmd := &cobra.Command{Use: "errors", Short: "Inspect accumulated operational errors without opening conversation contents", Args: cobra.NoArgs}
	list := func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		reports, err := errorreport.List(root, c.Limits)
		if err != nil {
			return err
		}
		if len(args) == 1 {
			for _, report := range reports {
				if report.ID == args[0] {
					return output(cmd, report)
				}
			}
			return errors.New("error report not found")
		}
		return output(cmd, reports)
	}
	cmd.RunE = list
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "List retained error groups", RunE: list})
	cmd.AddCommand(&cobra.Command{Use: "show ID", Args: cobra.ExactArgs(1), Short: "Show a selected error group and recent correlations", RunE: list})
	cmd.AddCommand(&cobra.Command{Use: "ack ID", Args: cobra.ExactArgs(1), Short: "Mark current occurrences reviewed; retain the error history", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		if err = errorreport.Acknowledge(cmd.Context(), root, args[0], c.Limits); err != nil {
			return err
		}
		return output(cmd, map[string]any{"acknowledged": args[0], "history_retained": true})
	}})
	return cmd
}
