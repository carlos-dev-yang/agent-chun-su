package cli

import (
	"context"
	"errors"

	"chunsu/internal/config"
	"chunsu/internal/selfupdate"
	"github.com/spf13/cobra"
)

func (o *options) update() *cobra.Command {
	cmd := &cobra.Command{Use: "update", Short: "Inspect or apply the owner-configured self-update"}
	cmd.AddCommand(&cobra.Command{Use: "configure --repository PATH", Short: "Record one clean master checkout and its origin for self-update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		repository, _ := cmd.Flags().GetString("repository")
		goPath, _ := cmd.Flags().GetString("go-path")
		gitPath, _ := cmd.Flags().GetString("git-path")
		installedRevision, _ := cmd.Flags().GetString("installed-revision")
		if repository == "" {
			return errors.New("--repository is required")
		}
		root, err := o.path()
		if err != nil {
			return err
		}
		settings, err := selfupdate.Configure(cmd.Context(), root, repository, goPath, gitPath, installedRevision, config.DefaultMaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"configured": true, "installed_revision": settings.InstalledRevision})
	}})
	configure := cmd.Commands()[0]
	configure.Flags().String("repository", "", "Trusted local checkout; must be a clean master branch with an origin remote")
	configure.Flags().String("go-path", "", "Explicit Go executable used to build the candidate")
	configure.Flags().String("git-path", "", "Explicit Git executable used to verify and fetch the configured checkout")
	configure.Flags().String("installed-revision", "", "Verified source revision of the currently installed command")
	cmd.AddCommand(&cobra.Command{Use: "check", Short: "Fetch the configured origin master and report whether an update is available", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		result, err := selfupdate.Check(cmd.Context(), root, config.DefaultMaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(cmd, result)
	}})
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Read durable self-update state without fetching or changing services", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		result, err := selfupdate.Inspect(root, config.DefaultMaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(cmd, result)
	}})
	cmd.AddCommand(&cobra.Command{Use: "metadata", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		value, err := selfupdate.SelfMetadata()
		if err != nil {
			return err
		}
		return output(cmd, value)
	}})
	cmd.AddCommand(&cobra.Command{Use: "apply", Short: "Queue the configured update in an independent user service", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		id, err := selfupdate.LaunchManual(cmd.Context(), root)
		if err != nil {
			return err
		}
		return output(cmd, map[string]string{"request_id": id, "status": "accepted"})
	}})
	var id string
	var updateID, chatID int64
	run := &cobra.Command{Use: "run", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if id == "" || updateID < 0 || (chatID == 0 && updateID != 0) || (chatID > 0 && updateID == 0) {
			return errors.New("independent update request is incomplete")
		}
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		runCtx, cancel := context.WithTimeout(cmd.Context(), selfupdate.RunTimeout)
		defer cancel()
		result, err := selfupdate.Run(runCtx, root, id, updateID, chatID, c)
		if err != nil {
			return err
		}
		return output(cmd, result)
	}}
	run.Flags().StringVar(&id, "id", "", "")
	run.Flags().Int64Var(&updateID, "update-id", -1, "")
	run.Flags().Int64Var(&chatID, "chat-id", 0, "")
	_ = run.Flags().MarkHidden("id")
	_ = run.Flags().MarkHidden("update-id")
	_ = run.Flags().MarkHidden("chat-id")
	cmd.AddCommand(run)
	cmd.AddCommand(&cobra.Command{Use: "chat-quiesce", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return quiesceTelegramForUpdate(cmd.Context(), root, c)
	}})
	cmd.AddCommand(&cobra.Command{Use: "chat-resume", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return resumeTelegramForUpdate(cmd.Context(), root, c)
	}})
	return cmd
}
