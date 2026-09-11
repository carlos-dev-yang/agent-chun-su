package cli

import (
	"errors"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/platform"
	"chunsu/internal/runtimeenv"
	"github.com/spf13/cobra"
)

func (o *options) configRoute() *cobra.Command {
	var driver, path, model, environment string
	var inherit bool
	cmd := &cobra.Command{Use: "route ROLE", Short: "Select the task, reception or review driver independently", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		role := args[0]
		if role != config.RoleTask && role != config.RoleReception && role != config.RoleReview {
			return errors.New("role must be task, reception or review")
		}
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		lock, err := platform.Acquire(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		c, err = config.Load(root)
		if err != nil {
			return err
		}
		if inherit {
			if role == config.RoleTask || driver != "" || path != "" || model != "" || environment != "" {
				return errors.New("inherit applies only to reception/review and cannot be combined with route fields")
			}
			if role == config.RoleReception {
				c.Routes.Reception = nil
			} else {
				c.Routes.Review = nil
			}
		} else {
			if driver == "" && path == "" && model == "" && environment == "" {
				return output(cmd, map[string]any{"role": role, "executor": c.ExecutorFor(role)})
			}
			selected := c.ExecutorFor(role)
			if driver != "" {
				selected.Kind = driver
			}
			if path != "" {
				selected.Path = path
			}
			if model != "" {
				selected.Model = model
			}
			if environment != "" {
				selected.Environment = environment
			}
			if _, err = executor.Select(selected.Kind); err != nil {
				return err
			}
			if _, err = runtimeenv.Select(selected.Environment); err != nil {
				return err
			}
			selected.RevokeDisclosure()
			switch role {
			case config.RoleTask:
				c.Executor = selected
			case config.RoleReception:
				c.Routes.Reception = &selected
			case config.RoleReview:
				c.Routes.Review = &selected
			}
		}
		if err = config.Save(root, c); err != nil {
			return err
		}
		return output(cmd, map[string]any{"role": role, "executor": c.ExecutorFor(role), "inherit_task": inherit, "disclosure": "an explicitly changed route requires its own live-data approval"})
	}}
	cmd.Flags().StringVar(&driver, "driver", "", "Compiled AI driver")
	cmd.Flags().StringVar(&path, "path", "", "Driver executable path")
	cmd.Flags().StringVar(&model, "model", "", "Model selected for this role")
	cmd.Flags().StringVar(&environment, "environment", "", "Execution environment adapter")
	cmd.Flags().BoolVar(&inherit, "inherit", false, "Use task route again (reception/review only)")
	return cmd
}
