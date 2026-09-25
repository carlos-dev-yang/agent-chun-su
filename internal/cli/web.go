package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/platform"
	"chunsu/internal/secrets"
	"chunsu/internal/webresearch"
	"github.com/spf13/cobra"
)

func (o *options) web() *cobra.Command {
	cmd := &cobra.Command{Use: "web", Short: "Configure public read-only web research"}
	cmd.AddCommand(&cobra.Command{Use: "key", Short: "Store a Brave Search API key with masked local input", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		if _, err := config.Load(root); err != nil {
			return err
		}
		if err := secrets.BootstrapLinuxStore(root); err != nil {
			return err
		}
		store, err := secrets.Open()
		if err != nil {
			return err
		}
		fmt.Fprint(command.OutOrStdout(), "Brave Search API key (hidden): ")
		key, err := platform.ReadSecret(command.Context(), secrets.MaxSecretBytes)
		fmt.Fprintln(command.OutOrStdout())
		if err != nil {
			return err
		}
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key, "\r\n\x00") {
			return errors.New("invalid Brave Search API key")
		}
		if err := store.Set(command.Context(), webresearch.KeyRef(root), key); err != nil {
			return err
		}
		fmt.Fprintln(command.OutOrStdout(), "Brave key saved in the host credential store; it was not added to config or chat history.")
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "limit NUMBER", Short: "Set the UTC daily search-attempt cap", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		value, err := strconv.Atoi(args[0])
		if err != nil {
			return errors.New("daily web search limit must be an integer")
		}
		lock, err := platform.Acquire(command.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		settings, err := webresearch.LoadSettings(root)
		if err != nil {
			return err
		}
		settings.SearchesPerDay = value
		if err := webresearch.SaveSettings(root, settings); err != nil {
			return err
		}
		fmt.Fprintf(command.OutOrStdout(), "Daily web search limit: %d attempted requests (UTC).\n", settings.SearchesPerDay)
		return nil
	}})
	return cmd
}
