package cli

import (
	"errors"
	"time"

	"chunsu/internal/gmail"
	"chunsu/internal/mail"
	"github.com/spf13/cobra"
)

func (o *options) gmailQuery() *cobra.Command {
	var asOf, zone string
	cmd := &cobra.Command{Use: "query QUERY", Args: cobra.ExactArgs(1), Short: "Preview the pinned Gmail query without authentication or collection", RunE: func(cmd *cobra.Command, args []string) error {
		if asOf == "" || zone == "" {
			return errors.New("explicit --as-of and --timezone are required")
		}
		at, err := time.Parse(time.RFC3339Nano, asOf)
		if err != nil {
			return err
		}
		query, err := gmail.ResolveQuery(args[0], at, zone)
		if err != nil {
			return err
		}
		return output(cmd, map[string]string{"original_query": args[0], "effective_query": query, "as_of": at.UTC().Format(time.RFC3339Nano), "timezone": zone, "relative_date_policy": "calendar units in the report timezone, pinned once per acquisition chain"})
	}}
	cmd.Flags().StringVar(&asOf, "as-of", "", "RFC3339 time boundary")
	cmd.Flags().StringVar(&zone, "timezone", "", "IANA report timezone")
	return cmd
}

func (o *options) gmailHistory() *cobra.Command {
	return &cobra.Command{Use: "history SNAPSHOT.json", Args: cobra.ExactArgs(1), Short: "Inspect bounded prior local reports for an existing source envelope; no account access", RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		b, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		snapshot, err := mail.ParseSnapshot(b, c.Limits)
		if err != nil {
			return err
		}
		prior, gaps, err := (gmail.Collector{Store: s, Config: c}).PriorReports(cmd.Context(), snapshot)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"prior_interpretations": prior, "gaps": gaps, "scope": "only earlier local reports whose sources are all in this envelope"})
	}}
}
