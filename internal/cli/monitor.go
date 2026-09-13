package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/opsmonitor"
	"chunsu/internal/service"
	"github.com/spf13/cobra"
)

func (o *options) monitor() *cobra.Command {
	cmd := &cobra.Command{Use: "monitor", Short: "Observe chat flow facts and retain local alert transitions"}
	cmd.AddCommand(&cobra.Command{Use: "check", Aliases: []string{"inspect"}, Args: cobra.NoArgs, Short: "Read current flow facts without persisting or changing processes", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return output(cmd, opsmonitor.Check(cmd.Context(), root, c))
	}})
	var once, managed bool
	run := &cobra.Command{Use: "run", Args: cobra.NoArgs, Short: "Persist foreground monitor observations until stopped", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return opsmonitor.Run(cmd.Context(), root, c, once, managed)
	}}
	run.Flags().BoolVar(&once, "once", false, "Persist one observation and exit")
	run.Flags().BoolVar(&managed, "managed", false, "Honor monitor service enabled intent")
	_ = run.Flags().MarkHidden("managed")
	cmd.AddCommand(run)
	cmd.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, Short: "Read saved monitor state, alert transitions, and process freshness", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		status, err := opsmonitor.ReadStatus(cmd.Context(), root, c)
		if err != nil {
			return err
		}
		return output(cmd, status)
	}})
	for _, operation := range []string{"render", "enable", "start", "stop", "restart", "disable", "remove"} {
		var atLogin bool
		child := &cobra.Command{Use: operation, Args: cobra.NoArgs, Short: "Manage monitor service: " + operation, RunE: func(cmd *cobra.Command, args []string) error {
			root, err := o.path()
			if err != nil {
				return err
			}
			c, err := config.Load(root)
			if err != nil {
				return err
			}
			timeout := time.Duration(c.Limits.LockWaitSeconds) * time.Second
			if operation == "render" {
				definition, err := service.RenderFor(root, service.Monitor, atLogin)
				if err != nil {
					return err
				}
				if o.json {
					return output(cmd, definition)
				}
				_, err = fmt.Fprint(cmd.OutOrStdout(), definition.Body)
				return err
			}
			registered := false
			if _, readErr := service.ReadFor(root, service.Monitor); readErr == nil {
				registered = true
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return readErr
			}
			if operation == "stop" || operation == "disable" || operation == "restart" || operation == "remove" {
				if err = service.SetMonitorEnabled(root, false); err != nil {
					return err
				}
				if registered {
					status, commandErr := service.CommandFor(cmd.Context(), root, service.Monitor, "status", timeout)
					if commandErr != nil {
						return commandErr
					}
					if status.Running {
						if _, err = service.CommandFor(cmd.Context(), root, service.Monitor, "stop", timeout); err != nil {
							return err
						}
					}
				}
				if operation == "remove" {
					if !registered {
						return output(cmd, map[string]any{"removed": false, "data_preserved": true})
					}
					definition, err := service.RemoveFor(cmd.Context(), root, service.Monitor, timeout)
					if err != nil {
						return err
					}
					return output(cmd, map[string]any{"removed": definition.Path, "data_preserved": true})
				}
				if operation != "restart" {
					return output(cmd, map[string]any{"enabled": false, "explicit_stop_saved": true})
				}
			}
			if operation == "enable" {
				if _, err = service.InstallFor(root, service.Monitor, atLogin); err != nil {
					return err
				}
				registered = true
			}
			if !registered {
				return errors.New("install the monitor with chunsu monitor enable")
			}
			if err = service.SetMonitorEnabled(root, true); err != nil {
				return err
			}
			result, err := service.CommandFor(cmd.Context(), root, service.Monitor, "start", timeout)
			if err != nil {
				_ = output(cmd, result)
				return err
			}
			return output(cmd, map[string]any{"enabled": true, "service": result.Label, "next": "chunsu monitor status", "diagnostics": "chunsu monitor check"})
		}}
		if operation == "enable" || operation == "render" {
			child.Flags().BoolVar(&atLogin, "at-login", true, "Start at user login; on Linux a persistent user manager is required after logout/reboot")
		}
		cmd.AddCommand(child)
	}
	return cmd
}
