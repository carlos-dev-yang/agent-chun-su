package cli

import (
	"errors"
	"strings"
	"time"

	"chunsu/internal/control"
	"chunsu/internal/gmail"
	"chunsu/internal/runner"
	"chunsu/internal/schedule"
	"github.com/spf13/cobra"
)

func (o *options) queueControl(operation string) *cobra.Command {
	return &cobra.Command{Use: operation, Short: "Inspect or control admission of local queued work", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		req := control.Request{Operation: operation}
		if handled, err := o.managed(cmd, req); handled || err != nil {
			return err
		}
		s, c, close, err := o.open(cmd.Context(), operation != "status")
		if err != nil {
			return err
		}
		defer close()
		if operation == "status" {
			paused, err := s.Paused(cmd.Context())
			if err != nil {
				return err
			}
			jobs, err := s.Jobs(cmd.Context())
			if err != nil {
				return err
			}
			counts := map[string]int{}
			for _, j := range jobs {
				counts[j.Status]++
			}
			return output(cmd, map[string]any{"owner": "not running", "queue_paused": paused, "jobs_by_status": counts})
		}
		data, err := (&runner.Runner{Store: s, Config: c}).Handle(cmd.Context(), req)
		if err != nil {
			return err
		}
		return output(cmd, data)
	}}
}

func (o *options) schedule() *cobra.Command {
	cmd := &cobra.Command{Use: "schedule", Short: "Manage opt-in elapsed-interval Gmail schedules (stop the worker to edit)"}
	var interval time.Duration
	var name string
	add := &cobra.Command{Use: "add CONNECTION_ID", Short: "Create a disabled schedule using the connection's report timezone", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		connection, err := gmail.ReadConnection(s.Root, args[0], c)
		if err != nil {
			return err
		}
		v, err := s.AddSchedule(cmd.Context(), connection.ID, strings.TrimSpace(name), connection.Policy.Timezone, interval)
		if err != nil {
			return err
		}
		return output(cmd, v)
	}}
	add.Flags().DurationVar(&interval, "every", 0, "Required elapsed interval, such as 8h or 24h; not a wall-clock appointment")
	add.Flags().StringVar(&name, "name", "", "Required human-readable schedule name")
	cmd.AddCommand(add)
	for _, operation := range []string{"enable", "disable"} {
		cmd.AddCommand(&cobra.Command{Use: operation + " SCHEDULE_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			s, c, close, err := o.open(cmd.Context(), true)
			if err != nil {
				return err
			}
			defer close()
			v, err := s.Schedule(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if operation == "enable" {
				if _, err = gmail.LoadConnection(s.Root, v.ConnectionID, c); err != nil {
					return err
				}
				if c.Executor.Path == "" || c.Executor.Kind == "" {
					return errors.New("select and validate the executor before enabling scheduling")
				}
			}
			if err = s.EnableSchedule(cmd.Context(), v.ID, operation == "enable"); err != nil {
				return err
			}
			v, err = s.Schedule(cmd.Context(), v.ID)
			if err != nil {
				return err
			}
			return output(cmd, v)
		}})
	}
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		v, err := s.Schedules(cmd.Context())
		if err != nil {
			return err
		}
		return output(cmd, v)
	}})
	cmd.AddCommand(&cobra.Command{Use: "ticks", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		v, err := s.Ticks(cmd.Context())
		if err != nil {
			return err
		}
		return output(cmd, v)
	}})
	cmd.AddCommand(&cobra.Command{Use: "retry TICK_ID", Args: cobra.ExactArgs(1), Short: "Retry blocked collection after repair, preserving its original acquisition", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		v, err := (schedule.Manager{Store: s, Config: c}).Retry(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return output(cmd, v)
	}})
	cmd.AddCommand(&cobra.Command{Use: "skip TICK_ID REASON", Args: cobra.ExactArgs(2), Short: "Explicitly abandon an unfinished occurrence and its pagination chain", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		v, err := (schedule.Manager{Store: s, Config: c}).Acknowledge(cmd.Context(), args[0], strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		return output(cmd, v)
	}})
	return cmd
}
