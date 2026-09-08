package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/jira"
	"chunsu/internal/runner"
	"chunsu/internal/schedule"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
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
	cmd := &cobra.Command{Use: "schedule", Short: "Manage opt-in elapsed-interval schedules (stop the worker to edit)"}
	var interval time.Duration
	var name string
	add := &cobra.Command{Use: "add CONNECTION_ID", Short: "Create a disabled schedule using the connection's report timezone", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		connectionID, timezone, err := scheduleProfile(s.Root, args[0], c)
		if err != nil {
			return err
		}
		v, err := s.AddSchedule(cmd.Context(), connectionID, strings.TrimSpace(name), timezone, interval)
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
				if err = validateScheduleEnable(cmd.Context(), s, v, c); err != nil {
					return err
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

func scheduleProfile(root, id string, c config.Config) (string, string, error) {
	path, err := jira.ProfilePath(id)
	if err != nil {
		return "", "", err
	}
	if _, err = os.Stat(filepath.Join(root, path)); err == nil {
		profile, err := jira.ReadProfile(root, id, c)
		if err != nil {
			return "", "", err
		}
		return profile.ID, profile.Timezone, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	connection, err := gmail.ReadConnection(root, id, c)
	if err != nil {
		return "", "", err
	}
	return connection.ID, connection.Policy.Timezone, nil
}

func validateScheduleEnable(ctx context.Context, s *store.Store, schedule store.Schedule, c config.Config) error {
	root := s.Root
	path, err := jira.ProfilePath(schedule.ConnectionID)
	if err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(root, path)); err == nil {
		profile, err := jira.LoadProfile(root, schedule.ConnectionID, c)
		if err != nil {
			return err
		}
		if profile.Timezone != schedule.Timezone {
			return errors.New("Jira profile timezone changed; review and replace the schedule")
		}
		if !c.Executor.LiveJiraApproved || c.Executor.LiveJiraPolicyDigest != profile.ReportPolicyDigest() {
			return errors.New("approve Jira live disclosure after validating the Jira executor boundary before enabling scheduling")
		}
		proof, err := activeJiraProofPackage(root, c, profile)
		if err != nil {
			return err
		}
		if err = runner.VerifyJiraLiveProof(ctx, s, c, proof); err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		connection, err := gmail.LoadConnection(root, schedule.ConnectionID, c)
		if err != nil {
			return err
		}
		if connection.Policy.Timezone != schedule.Timezone {
			return errors.New("connection timezone changed; review and replace the schedule")
		}
	} else {
		return err
	}
	if c.Executor.Path == "" || c.Executor.Kind == "" {
		return errors.New("select and validate the executor before enabling scheduling")
	}
	return nil
}

func activeJiraProofPackage(root string, c config.Config, profile jira.Profile) (workgroup.Package, error) {
	bundle, digest, err := workgroup.ActiveFor(root, "jira-report", c.Limits.MaxArtifactBytes)
	if err != nil {
		return workgroup.Package{}, err
	}
	skill, err := bundle.SelectedSkill()
	if err != nil {
		return workgroup.Package{}, err
	}
	return workgroup.Package{
		Workgroup:       "jira-report",
		WorkgroupDigest: digest,
		Skill: workgroup.SkillIdentity{
			Name:        skill.Name,
			Description: skill.Description,
			Digest:      files.Digest([]byte(skill.Markdown)),
		},
		JiraSnapshot: &jira.ReportInput{ReportPolicyDigest: profile.ReportPolicyDigest(), Snapshot: jira.Snapshot{Synthetic: false}},
	}, nil
}
