package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/mail"
	"chunsu/internal/runner"
	"chunsu/internal/secrets"
	"github.com/spf13/cobra"
)

func (o *options) gmail() *cobra.Command {
	cmd := &cobra.Command{Use: "gmail", Short: "Connect one scoped Gmail account and collect read-only mail snapshots"}
	cmd.AddCommand(o.gmailConnect(false), o.gmailConnect(true), o.gmailCollect(false), o.gmailCollect(true))
	cmd.AddCommand(o.gmailQuery(), o.gmailHistory())
	cmd.AddCommand(&cobra.Command{Use: "keychain-check", Short: "Write, verify and delete a random non-production Keychain marker", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) (result error) {
		k, err := secrets.Open()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(gmail.DefaultHTTPTimeoutSeconds)*time.Second)
		defer cancel()
		ref := files.ID()
		marker := files.ID()
		cleanupNeeded := true
		defer func() {
			if !cleanupNeeded {
				return
			}
			cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(gmail.DefaultHTTPTimeoutSeconds)*time.Second)
			defer done()
			if err := k.Delete(cleanup, ref); err != nil {
				result = errors.Join(result, fmt.Errorf("remove non-production Keychain marker: %w", err))
			}
		}()
		if err = k.Set(ctx, ref, marker); err != nil {
			return err
		}
		if err = k.Delete(ctx, ref); err != nil {
			return err
		}
		cleanupNeeded = false
		if _, err = k.Get(ctx, ref); !errors.Is(err, secrets.ErrMissing) || ctx.Err() != nil {
			return errors.Join(errors.New("Keychain marker deletion could not be verified"), err, ctx.Err())
		}
		return output(cmd, map[string]any{"keychain": "verified", "material": "random non-production marker", "deleted": true})
	}})
	cmd.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(root, "state", "connections"))
		if errors.Is(err, os.ErrNotExist) {
			return output(cmd, []any{})
		}
		if err != nil {
			return err
		}
		out := []gmail.Connection{}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			connection, e := gmail.ReadConnection(root, strings.TrimSuffix(entry.Name(), ".json"), c)
			if e != nil {
				return e
			}
			out = append(out, connection)
		}
		return output(cmd, out)
	}})
	cmd.AddCommand(&cobra.Command{Use: "check CONNECTION_ID", Short: "Verify token refresh and selected account without reading message bodies", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		connection, err := gmail.LoadConnection(s.Root, args[0], c)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(gmail.DefaultHTTPTimeoutSeconds)*time.Second)
		defer cancel()
		client, err := gmail.OpenClient(ctx, connection, c)
		if err != nil {
			return err
		}
		account, err := client.Profile(ctx)
		if err != nil {
			return err
		}
		if err = gmail.CheckAccount(connection.Account, account); err != nil {
			return err
		}
		return output(cmd, map[string]any{"connection_id": connection.ID, "account": account, "scopes": connection.Scopes, "policy": connection.Policy, "message_bodies_read": false})
	}})
	cmd.AddCommand(&cobra.Command{Use: "acquisitions", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		acquisitions, err := s.Acquisitions(cmd.Context(), "")
		if err != nil {
			return err
		}
		return output(cmd, acquisitions)
	}})
	cmd.AddCommand(&cobra.Command{Use: "queue ACQUISITION_ID", Short: "Queue a collected snapshot exactly once by its acquisition identity", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		j, err := (gmail.Collector{Store: s, Config: c}).Queue(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return output(cmd, j)
	}})
	var revoke bool
	disconnect := &cobra.Command{Use: "disconnect CONNECTION_ID", Short: "Disable local access and remove credential references; preserve local evidence", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		connection, err := gmail.ReadConnection(s.Root, args[0], c)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(gmail.DefaultHTTPTimeoutSeconds)*time.Second)
		defer cancel()
		if err = gmail.Disconnect(ctx, s.Root, connection, revoke); err != nil {
			return err
		}
		return output(cmd, map[string]any{"connection_id": connection.ID, "enabled": false, "remote_revocation": revoke, "local_evidence": "preserved"})
	}}
	disconnect.Flags().BoolVar(&revoke, "revoke", false, "Also revoke this OAuth grant with Google")
	cmd.AddCommand(disconnect, o.gmailNormalize())
	return cmd
}

func (o *options) gmailConnect(again bool) *cobra.Command {
	var clientFile, account, query, zone string
	var batch, historyDays int
	var history, noBrowser bool
	use := "connect"
	argc := 0
	if again {
		use = "reauth CONNECTION_ID"
		argc = 1
	}
	cmd := &cobra.Command{Use: use, Short: "Authorize a Desktop app client using a temporary loopback callback and Keychain", Args: cobra.ExactArgs(argc), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		if clientFile == "" {
			return errors.New("provide --client with the local Google Desktop app client JSON path")
		}
		data, err := readExternal(clientFile, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		if zone == "" {
			zone = c.Timezone
		}
		policy := gmail.Policy{Query: query, BatchSize: batch, ThreadHistory: history, HistoryDays: historyDays, Retention: "manual", Timezone: zone}
		var previous *gmail.Connection
		if again {
			p, e := gmail.ReadConnection(s.Root, args[0], c)
			if e != nil {
				return e
			}
			previous = &p
			account = p.Account
			policy = p.Policy
		}
		if account == "" || policy.Query == "" {
			return errors.New("provide --account and an explicit --query for the pilot")
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Gmail pilot: %s; query %q; batch %d; thread history %t; timezone %s. Read-only; reports stay local; retention is manual.\n", account, policy.Query, policy.BatchSize, policy.ThreadHistory, policy.Timezone)
		connection, connectErr := gmail.Connect(cmd.Context(), s.Root, c, data, account, policy, previous, func(authURL string) error {
			if noBrowser {
				fmt.Fprintln(cmd.ErrOrStderr(), "Open this authorization URL in your system browser:\n"+authURL)
				return nil
			}
			opener, err := exec.LookPath("open")
			if err != nil {
				return errors.New("system browser opener is unavailable; retry with --no-browser")
			}
			return exec.CommandContext(cmd.Context(), opener, authURL).Run()
		})
		if connection.ID != "" {
			if err = output(cmd, connection); err != nil {
				return err
			}
		}
		return connectErr
	}}
	cmd.Flags().StringVar(&clientFile, "client", "", "Local Google OAuth Desktop app client JSON file")
	cmd.Flags().StringVar(&account, "account", "", "Exact Gmail account expected after authorization")
	cmd.Flags().StringVar(&query, "query", "", "Explicit Gmail search query; no broad implicit mailbox scan")
	cmd.Flags().IntVar(&batch, "batch", gmail.DefaultBatchSize, "Maximum listed messages in one batch")
	cmd.Flags().BoolVar(&history, "thread-history", false, "Also read bounded prior messages in the selected threads")
	cmd.Flags().IntVar(&historyDays, "history-days", gmail.DefaultHistoryDays, "Maximum age of related-thread reference history")
	cmd.Flags().StringVar(&zone, "timezone", "", "IANA report timezone; defaults to local configuration")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Print the authorization URL for opening in the system browser")
	return cmd
}

func (o *options) gmailCollect(review bool) *cobra.Command {
	var resume, continuation string
	var queue bool
	use := "collect CONNECTION_ID"
	if review {
		use = "review CONNECTION_ID"
	}
	cmd := &cobra.Command{Use: use, Short: "Collect a bounded Gmail snapshot; review also runs the configured AI executor", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		connection, err := gmail.LoadConnection(s.Root, args[0], c)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(c.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
		collector := gmail.Collector{Store: s, Config: c}
		acquisition, collectErr := collector.Collect(ctx, connection, resume, continuation)
		if collectErr != nil {
			_ = output(cmd, map[string]any{"acquisition": acquisition, "error": collectErr.Error()})
			return collectErr
		}
		if !queue && !review {
			return output(cmd, acquisition)
		}
		job, err := collector.Queue(cmd.Context(), acquisition.ID)
		if err != nil {
			return err
		}
		if !review {
			return output(cmd, map[string]any{"acquisition": acquisition, "job": job})
		}
		r := &runner.Runner{Store: s, Config: c}
		server, err := control.Listen(cmd.Context(), s.Root, c.Limits.MaxArtifactBytes, time.Duration(c.Limits.LockWaitSeconds)*time.Second, r.Handle)
		if err != nil {
			return err
		}
		defer server.Close()
		result, runErr := r.Run(cmd.Context(), job.ID, "")
		if err = output(cmd, map[string]any{"acquisition": acquisition, "result": result}); err != nil {
			return err
		}
		return runErr
	}}
	cmd.Flags().StringVar(&resume, "resume", "", "Resume an interrupted acquisition with the same pinned policy")
	cmd.Flags().StringVar(&continuation, "continue", "", "Continue the next page of a prior collected batch at its original as-of time")
	if !review {
		cmd.Flags().BoolVar(&queue, "queue", false, "Also queue the preserved snapshot for analysis")
	}
	return cmd
}

func (o *options) gmailNormalize() *cobra.Command {
	var connection, asOf, scope string
	cmd := &cobra.Command{Use: "normalize MESSAGE.json", Short: "Inspect normalization of a saved Gmail API message; does not connect an account", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c := config.Defaults()
		data, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		var raw gmail.APIMessage
		if err = json.Unmarshal(data, &raw); err != nil {
			return err
		}
		at, err := time.Parse(time.RFC3339, asOf)
		if err != nil {
			return errors.New("provide --as-of as an RFC3339 timestamp")
		}
		if scope != mail.Target && scope != mail.Reference {
			return errors.New("scope must be target or reference")
		}
		normalized, err := gmail.Normalize(connection, raw, scope, at, c.Limits)
		if err != nil {
			return err
		}
		return output(cmd, normalized)
	}}
	cmd.Flags().StringVar(&connection, "connection-id", "", "Source namespace for the saved message")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Pinned RFC3339 time boundary")
	cmd.Flags().StringVar(&scope, "scope", mail.Target, "target or reference")
	return cmd
}
