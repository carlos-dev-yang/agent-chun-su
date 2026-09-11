package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"chunsu/internal/access"
	"chunsu/internal/audit"
	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/runner"
	"github.com/spf13/cobra"
)

func (o *options) access() *cobra.Command {
	cmd := &cobra.Command{Use: "access", Short: "Inspect OS ownership or suspend and resume AI execution on this data home"}
	cmd.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		info, err := os.Lstat(root)
		if err != nil {
			return err
		}
		owner, err := files.OwnerUID(info)
		if err != nil {
			return err
		}
		routes := map[string]any{}
		for _, role := range []string{config.RoleReception, config.RoleTask, config.RoleReview} {
			e := c.ExecutorFor(role)
			routes[role] = map[string]bool{"mail": e.LiveMailApproved, "jira": e.LiveJiraApproved, "code": e.LiveCodeApproved}
		}
		return output(cmd, map[string]any{"data_home": root, "owner_os_user_id": owner, "caller_os_user_id": os.Geteuid(), "management": "private Unix socket; matching OS peer required", "ai_execution_blocked": access.Check(root) != nil, "route_disclosure": routes, "shared_multi_user": "not supported within one data home"})
	}})
	for _, operation := range []string{"revoke", "resume"} {
		cmd.AddCommand(&cobra.Command{Use: operation, Short: "Explicit owner action; resume does not restore source disclosure approvals", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			request := control.Request{Operation: operation + "_access"}
			if handled, err := o.managed(cmd, request); handled || err != nil {
				return err
			}
			s, c, close, err := o.open(cmd.Context(), true)
			if err != nil {
				return err
			}
			defer close()
			result, err := (&runner.Runner{Store: s, Config: c}).Handle(cmd.Context(), request)
			if err != nil {
				return err
			}
			return output(cmd, result)
		}})
	}
	var limit int
	history := &cobra.Command{Use: "history", Short: "Read bounded host ownership and control-action receipts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		if limit <= 0 || limit > c.Limits.MaxMessages {
			return errors.New("history limit must be within the configured source-count budget")
		}
		entries, err := os.ReadDir(filepath.Join(root, audit.Directory))
		if errors.Is(err, os.ErrNotExist) {
			return output(cmd, []audit.Event{})
		}
		if err != nil {
			return err
		}
		events := []audit.Event{}
		if len(entries) > limit {
			entries = entries[len(entries)-limit:]
		}
		for index := len(entries) - 1; index >= 0; index-- {
			entry := entries[index]
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return errors.New("invalid audit entry")
			}
			data, e := files.Read(root, filepath.Join(audit.Directory, entry.Name()), c.Limits.MaxSourceBytes)
			if e != nil {
				return e
			}
			var event audit.Event
			if e = json.Unmarshal(data, &event); e != nil {
				return e
			}
			events = append(events, event)
		}
		return output(cmd, events)
	}}
	history.Flags().IntVar(&limit, "limit", 1, "Most recent receipts to show, within the configured source-count budget")
	cmd.AddCommand(history)
	return cmd
}
