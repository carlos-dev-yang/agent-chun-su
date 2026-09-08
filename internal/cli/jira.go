package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"github.com/spf13/cobra"
)

func jiraSummary(r jira.Record) map[string]any {
	return map[string]any{
		"acquisition_id": r.State.ID, "provider": jira.Provider, "synthetic": r.State.Synthetic,
		"status": r.State.Status, "stop_reason": r.State.StopReason, "retrieval_complete": r.State.RetrievalComplete,
		"snapshot_status": r.Snapshot.Status, "pages_preserved": len(r.State.Pages), "rows_processed": r.State.Rows,
		"issues": len(r.Snapshot.Issues), "gaps": r.Snapshot.Gaps, "reprocess_of": r.State.ReprocessOf,
	}
}

func (o *options) jira() *cobra.Command {
	cmd := &cobra.Command{Use: "jira", Short: "Collect and inspect bounded Jira evidence"}
	cmd.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, Short: "Inspect reader readiness without account access", RunE: func(cmd *cobra.Command, args []string) error {
		return output(cmd, map[string]any{"provider": jira.Provider, "saved_reader": "ready", "live_reader": "jira-cloud profile required", "report_execution": "requires jira-report workgroup"})
	}})
	cmd.AddCommand(o.jiraConnect(), o.jiraCheck(), o.jiraDisconnect(), o.jiraConnections(), o.jiraCollectLive(), o.jiraResumeLive())
	cmd.AddCommand(&cobra.Command{Use: "inspect SAVED_RESPONSES.json", Args: cobra.ExactArgs(1), Short: "Validate the saved-input envelope with default byte limits; do not persist data", RunE: func(cmd *cobra.Command, args []string) error {
		data, err := readExternal(args[0], config.Defaults().Limits.MaxEvidenceBytes)
		if err != nil {
			return err
		}
		r, err := jira.NewSavedReader(data, config.Defaults().Limits)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"valid": true, "provider": jira.Provider, "format": r.Input.Format, "synthetic": r.Input.Synthetic, "saved_pages": len(r.Input.Pages), "source_digest": r.Digest, "policy": r.Input.Policy, "scope": "envelope and page validation; use collect for normalization"})
	}})
	cmd.AddCommand(&cobra.Command{Use: "collect [SAVED_RESPONSES.json]", Args: cobra.MaximumNArgs(1), Short: "Preserve and normalize a bounded saved-input acquisition", RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			_, err := (jira.DisconnectedReader{}).ReadPage(cmd.Context(), jira.Request{})
			if e := output(cmd, map[string]any{"provider": jira.Provider, "status": "not_configured"}); e != nil {
				return e
			}
			return err
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		data, err := readExternal(args[0], c.Limits.MaxEvidenceBytes)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		r, err := (jira.Collector{Store: s, Config: c}).CollectSaved(ctx, data)
		if r.State.ID != "" {
			if e := output(cmd, jiraSummary(r)); e != nil {
				return e
			}
		}
		return err
	}})
	cmd.AddCommand(&cobra.Command{Use: "resume ACQUISITION_ID", Args: cobra.ExactArgs(1), Short: "Continue the same pinned source, policy and mappings for another bounded page batch", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		r, err := (jira.Collector{Store: s, Config: c}).Resume(ctx, args[0])
		if r.State.ID != "" {
			if e := output(cmd, jiraSummary(r)); e != nil {
				return e
			}
		}
		return err
	}})
	var mappingFile string
	renormalize := &cobra.Command{Use: "renormalize ACQUISITION_ID --mapping MAPPING.json", Args: cobra.ExactArgs(1), Short: "Start a new derived acquisition from preserved bytes with an explicit field mapping", RunE: func(cmd *cobra.Command, args []string) error {
		if mappingFile == "" {
			return errors.New("provide --mapping with a versioned Jira field mapping")
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		data, err := readExternal(mappingFile, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		var mapping jira.Mapping
		// Match strict decoding of all other user-owned Jira configuration.
		mapping, err = jira.ParseMapping(data)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		r, err := (jira.Collector{Store: s, Config: c}).Renormalize(ctx, args[0], mapping)
		if r.State.ID != "" {
			if e := output(cmd, jiraSummary(r)); e != nil {
				return e
			}
		}
		return err
	}}
	renormalize.Flags().StringVar(&mappingFile, "mapping", "", "Versioned field mapping JSON; previous evidence remains immutable")
	cmd.AddCommand(renormalize)
	cmd.AddCommand(&cobra.Command{Use: "acquisitions", Args: cobra.NoArgs, Short: "List only Jira acquisition checkpoints", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		all, err := s.AcquisitionsByProvider(cmd.Context(), jira.Provider, c.Limits.MaxEvidenceBytes)
		if err != nil {
			return err
		}
		return output(cmd, all)
	}})
	cmd.AddCommand(&cobra.Command{Use: "queue ACQUISITION_ID", Args: cobra.ExactArgs(1), Short: "Queue one immutable Jira report input from an available live acquisition", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		job, err := (jira.Collector{Store: s, Config: c}).Queue(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return output(cmd, job)
	}})
	var snapshot bool
	show := &cobra.Command{Use: "show ACQUISITION_ID", Args: cobra.ExactArgs(1), Short: "Verify and show pinned policy, raw references and progress", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		r, err := (jira.Collector{Store: s, Config: c}).Load(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if snapshot {
			return output(cmd, r.Snapshot)
		}
		return output(cmd, r.State)
	}}
	show.Flags().BoolVar(&snapshot, "snapshot", false, "Show the verified normalized snapshot instead of the checkpoint")
	cmd.AddCommand(show)
	cmd.AddCommand(&cobra.Command{Use: "source ACQUISITION_ID PAGE_INDEX", Args: cobra.ExactArgs(2), Short: "Print exact verified response bytes for a zero-based preserved page index", RunE: func(cmd *cobra.Command, args []string) error {
		index, err := strconv.Atoi(args[1])
		if err != nil {
			return errors.New("page index must be an integer")
		}
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		data, err := (jira.Collector{Store: s, Config: c}).Source(cmd.Context(), args[0], index)
		if err != nil {
			return err
		}
		if !json.Valid(data) {
			return errors.New("preserved response is not JSON")
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}})
	return cmd
}

func (o *options) jiraConnect() *cobra.Command {
	var id, siteHost, cloudID, account, project, board, todoID, todoName, dueField, startField, zone, service, keychainAccount, selection, content string
	var maxIssues, maxPages int
	cmd := &cobra.Command{Use: "connect", Args: cobra.NoArgs, Short: "Verify and save a read-only Jira Cloud profile using a user-owned Keychain item", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if id == "" {
			id = files.ID()
		}
		if zone == "" {
			zone = c.Timezone
		}
		if keychainAccount == "" {
			keychainAccount = account
		}
		profile := jira.Profile{Version: jira.Version, ID: id, Provider: jira.CloudProvider, SiteHost: siteHost, CloudID: cloudID, ExpectedAccount: account, ProjectKey: project, BoardID: board, TodoStatusID: todoID, TodoStatusName: todoName, DueField: dueField, StartField: startField, Timezone: zone, Secret: jira.SecretRef{Store: jira.KeychainSecretStore, Service: service, Account: keychainAccount}, Policy: jira.LivePolicy{Selection: selection, WindowDays: jira.ReportWindowDays, ContentScope: content, MaxIssues: maxIssues, MaxPages: maxPages}}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		profile, err = jira.Connect(ctx, profile, c)
		if err != nil {
			return err
		}
		if err = jira.SaveProfile(s.Root, profile, false); err != nil {
			return err
		}
		return output(cmd, profile)
	}}
	cmd.Flags().StringVar(&id, "connection-id", "", "Optional generated local connection ID")
	cmd.Flags().StringVar(&siteHost, "site-host", "", "Exact Jira site host without scheme or path")
	cmd.Flags().StringVar(&cloudID, "cloud-id", "", "Verified Jira Cloud ID")
	cmd.Flags().StringVar(&account, "account", "", "Exact expected Jira account email")
	cmd.Flags().StringVar(&project, "project-key", "", "Reviewed Jira project key")
	cmd.Flags().StringVar(&board, "board-id", "", "Reviewed positive Jira board ID")
	cmd.Flags().StringVar(&todoID, "todo-status-id", "", "Reviewed TODO status ID")
	cmd.Flags().StringVar(&todoName, "todo-status-name", "", "Observed TODO status display name")
	cmd.Flags().StringVar(&dueField, "due-field", "", "Reviewed due-date field ID")
	cmd.Flags().StringVar(&startField, "start-field", "", "Reviewed start-date field ID")
	cmd.Flags().StringVar(&zone, "timezone", "", "IANA report timezone")
	cmd.Flags().StringVar(&service, "keychain-service", "", "Existing macOS Keychain service name")
	cmd.Flags().StringVar(&keychainAccount, "keychain-account", "", "Existing macOS Keychain account; defaults to --account")
	cmd.Flags().StringVar(&selection, "selection", jira.GroupedSelection, "Reviewed Jira selection policy")
	cmd.Flags().StringVar(&content, "content-scope", "metadata_and_description", "metadata_only or metadata_and_description")
	cmd.Flags().IntVar(&maxIssues, "max-issues", config.DefaultMaxMessages, "Maximum issues per acquisition")
	cmd.Flags().IntVar(&maxPages, "max-pages", 5, "Maximum provider pages per acquisition")
	return cmd
}

func (o *options) jiraCheck() *cobra.Command {
	return &cobra.Command{Use: "check CONNECTION_ID", Args: cobra.ExactArgs(1), Short: "Reverify an active Jira Cloud identity, board-project binding and fields", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		profile, err := jira.LoadProfile(s.Root, args[0], c)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		profile, err = jira.Check(ctx, profile, c)
		if err != nil {
			return err
		}
		if err = jira.SaveProfile(s.Root, profile, true); err != nil {
			return err
		}
		return output(cmd, profile)
	}}
}
func (o *options) jiraDisconnect() *cobra.Command {
	return &cobra.Command{Use: "disconnect CONNECTION_ID", Args: cobra.ExactArgs(1), Short: "Disable a local Jira profile and clear its Keychain binding; preserve evidence", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		profile, err := jira.ReadProfile(s.Root, args[0], c)
		if err != nil {
			return err
		}
		if err = jira.Disconnect(s.Root, profile, c); err != nil {
			return err
		}
		return output(cmd, map[string]any{"connection_id": profile.ID, "enabled": false, "external_keychain_item": "preserved", "local_evidence": "preserved"})
	}}
}
func (o *options) jiraConnections() *cobra.Command {
	return &cobra.Command{Use: "connections", Args: cobra.NoArgs, Short: "List local Jira Cloud profiles without reading Keychain values", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		out := []jira.Profile{}
		entries, err := os.ReadDir(filepath.Join(root, "state", "connections", jira.CloudProvider))
		if errors.Is(err, os.ErrNotExist) {
			return output(cmd, out)
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			profile, e := jira.ReadProfile(root, strings.TrimSuffix(entry.Name(), ".json"), c)
			if e != nil {
				return e
			}
			out = append(out, profile)
		}
		return output(cmd, out)
	}}
}
func (o *options) jiraCollectLive() *cobra.Command {
	var queue bool
	cmd := &cobra.Command{Use: "collect-live CONNECTION_ID", Args: cobra.ExactArgs(1), Short: "Collect a bounded read-only Jira Cloud snapshot through a verified profile", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		profile, err := jira.LoadProfile(s.Root, args[0], c)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		record, err := (jira.Collector{Store: s, Config: c}).CollectLive(ctx, profile)
		if err != nil {
			return err
		}
		if !queue {
			return output(cmd, jiraSummary(record))
		}
		job, err := (jira.Collector{Store: s, Config: c}).Queue(cmd.Context(), record.State.ID)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"acquisition": jiraSummary(record), "job": job})
	}}
	cmd.Flags().BoolVar(&queue, "queue", false, "Queue the immutable jira-report input after collection")
	return cmd
}
func (o *options) jiraResumeLive() *cobra.Command {
	return &cobra.Command{Use: "resume-live CONNECTION_ID ACQUISITION_ID", Args: cobra.ExactArgs(2), Short: "Continue a bounded live Jira acquisition using the same verified connection", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		profile, err := jira.LoadProfile(s.Root, args[0], c)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		record, err := (jira.Collector{Store: s, Config: c}).ResumeLive(ctx, profile, args[1])
		if err != nil {
			return err
		}
		return output(cmd, jiraSummary(record))
	}}
}
