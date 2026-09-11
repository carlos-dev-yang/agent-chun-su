package cli

import (
	"context"
	"encoding/json"
	"time"

	"chunsu/internal/codereview"
	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/runner"
	"github.com/spf13/cobra"
)

func (o *options) code() *cobra.Command {
	cmd := &cobra.Command{Use: "code", Short: "Review explicitly selected committed repository files"}
	var base, head string
	var paths []string
	var synthetic bool
	capture := &cobra.Command{Use: "capture REPOSITORY", Short: "Snapshot selected Git files at two commits and queue a read-only review", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		input, err := codereview.Capture(ctx, args[0], base, head, paths, synthetic, c.Limits)
		if err != nil {
			return err
		}
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		request := control.Request{Operation: "queue", Workgroup: codereview.Workgroup, Input: data, SourceName: input.Repository}
		if handled, err := o.managed(cmd, request); handled || err != nil {
			return err
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		r := runner.Runner{Store: s, Config: c}
		job, err := r.Handle(ctx, request)
		if err != nil {
			return err
		}
		return output(cmd, job)
	}}
	capture.Flags().StringVar(&base, "base", "", "Existing base commit or ref; resolved to an immutable commit")
	capture.Flags().StringVar(&head, "head", "", "Existing head commit or ref; resolved to an immutable commit")
	capture.Flags().StringSliceVar(&paths, "path", nil, "Exact repository-relative regular file, repeat for more files")
	capture.Flags().BoolVar(&synthetic, "synthetic", false, "Declare that these are public synthetic examples, not private repository data")
	cmd.AddCommand(capture)
	return cmd
}
