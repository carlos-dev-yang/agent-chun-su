package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"chunsu/internal/config"
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
	cmd := &cobra.Command{Use: "jira", Short: "Collect and inspect Jira evidence from saved responses; live connection is deferred"}
	cmd.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, Short: "Inspect reader readiness without account access", RunE: func(cmd *cobra.Command, args []string) error {
		return output(cmd, map[string]any{"provider": jira.Provider, "saved_reader": "ready", "live_reader": "not_configured", "provider_selection": "pending", "report_execution": "deferred"})
	}})
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
