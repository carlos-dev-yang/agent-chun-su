package cli

import (
	"errors"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/runner"
	"chunsu/internal/schedule"
	"chunsu/internal/store"
	"github.com/spf13/cobra"
)

func (o *options) managed(cmd *cobra.Command, req control.Request) (bool, error) {
	root, err := o.path()
	if err != nil {
		return true, err
	}
	c, err := config.Load(root)
	if err != nil {
		return true, err
	}
	data, handled, err := control.Call(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, req)
	if err != nil || !handled {
		return handled, err
	}
	return true, output(cmd, data)
}

func (o *options) run() *cobra.Command {
	var candidate string
	cmd := &cobra.Command{Use: "run JOB_ID", Short: "Run one queued workgroup job and preserve its output and source evidence", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if _, err = runner.Recover(cmd.Context(), s, c); err != nil {
			return err
		}
		r := &runner.Runner{Store: s, Config: c}
		server, err := control.Listen(cmd.Context(), s.Root, c.Limits.MaxArtifactBytes, time.Duration(c.Limits.LockWaitSeconds)*time.Second, r.Handle)
		if err != nil {
			return err
		}
		defer server.Close()
		outcome, runErr := r.Run(cmd.Context(), args[0], candidate)
		if err = output(cmd, outcome); err != nil {
			return err
		}
		return runErr
	}}
	cmd.Flags().StringVar(&candidate, "candidate", "", "Use a stored candidate workgroup digest without adopting it")
	return cmd
}

func (o *options) resume(resolve bool) *cobra.Command {
	use, operation := "retry JOB_ID", "retry"
	argc := 1
	if resolve {
		use = "resolve JOB_ID ANSWER"
		operation = "resolve"
		argc = 2
	}
	return &cobra.Command{Use: use, Short: "Record explicit intervention and return eligible work to the queue", Args: cobra.ExactArgs(argc), RunE: func(cmd *cobra.Command, args []string) error {
		req := control.Request{Operation: operation, JobID: args[0]}
		if resolve {
			req.Answer = args[1]
		}
		if handled, err := o.managed(cmd, req); handled || err != nil {
			return err
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		r := &runner.Runner{Store: s, Config: c}
		data, err := r.Handle(cmd.Context(), req)
		if err != nil {
			return err
		}
		return output(cmd, data)
	}}
}

func (o *options) worker() *cobra.Command {
	var once bool
	cmd := &cobra.Command{Use: "worker", Short: "Own the local queue in the foreground; stop with Ctrl-C", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if c.Executor.Kind == "" || c.Executor.Path == "" {
			return errors.New("configure the executor before starting a worker")
		}
		if _, err = runner.Recover(cmd.Context(), s, c); err != nil {
			return err
		}
		r := &runner.Runner{Store: s, Config: c}
		server, err := control.Listen(cmd.Context(), s.Root, c.Limits.MaxArtifactBytes, time.Duration(c.Limits.LockWaitSeconds)*time.Second, r.Handle)
		if err != nil {
			return err
		}
		defer server.Close()
		ticker := time.NewTicker(time.Duration(c.Limits.PollSeconds) * time.Second)
		defer ticker.Stop()
		for {
			paused, err := s.Paused(cmd.Context())
			if err != nil {
				if cmd.Context().Err() != nil {
					return nil
				}
				return err
			}
			processed := false
			if !paused {
				jobs, err := s.Jobs(cmd.Context())
				if err != nil {
					if cmd.Context().Err() != nil {
						return nil
					}
					return err
				}
				for i := len(jobs) - 1; i >= 0; i-- {
					j := jobs[i]
					if (j.Status != store.Queued && j.Status != store.RetryWait) || j.NotBefore > time.Now().UnixMilli() {
						continue
					}
					outcome, runErr := r.Run(cmd.Context(), j.ID, "")
					processed = true
					if err = output(cmd, outcome); err != nil {
						return err
					}
					if runErr != nil && outcome.Status == "" {
						return runErr
					}
					if once {
						return runErr
					}
					break
				}
				if !processed {
					tick, e := (schedule.Manager{Store: s, Config: c}).Step(cmd.Context())
					if e != nil {
						if cmd.Context().Err() != nil {
							return nil
						}
						return e
					}
					if tick != nil {
						if e = output(cmd, tick); e != nil {
							return e
						}
					}
				}
			}
			if once {
				return nil
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-ticker.C:
			}
		}
	}}
	cmd.Flags().BoolVar(&once, "once", false, "Process at most one eligible attempt or scheduled collection and exit")
	return cmd
}
