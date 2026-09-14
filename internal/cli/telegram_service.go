package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"chunsu/internal/chatsupervisor"
	"chunsu/internal/config"
	"chunsu/internal/secrets"
	"chunsu/internal/service"
	"chunsu/internal/telegram"
	"github.com/spf13/cobra"
)

func (o *options) telegramStatus() *cobra.Command {
	return &cobra.Command{Use: "status", Args: cobra.NoArgs, Short: "Inspect pairing, observed receiver health and supervisor state", RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		binding, err := telegram.Load(root, c.Limits.MaxArtifactBytes)
		if errors.Is(err, os.ErrNotExist) {
			return output(cmd, map[string]any{"configured": false, "next": "chunsu telegram enable"})
		}
		if err != nil {
			return err
		}
		health, alive, healthErr := telegram.ReadHealth(root, c.Limits)
		supervisor, supervising, supervisorErr := chatsupervisor.Read(root, c.Limits)
		enabled, desiredErr := service.Enabled(root)
		offset, offsetErr := telegram.Offset(root, c.Limits.MaxArtifactBytes)
		result := map[string]any{"configured": true, "bot_username": binding.Bot.Username, "paired": binding.UserID > 0, "next_update": offset, "enabled": enabled, "receiver_alive": alive, "receiver": health, "supervisor_alive": supervising, "supervisor": supervisor, "state_readable": errors.Join(healthErr, supervisorErr, desiredErr, offsetErr) == nil}
		if err = output(cmd, result); err != nil {
			return err
		}
		return errors.Join(healthErr, supervisorErr, desiredErr, offsetErr)
	}}
}

func (o *options) addTelegramServiceCommands(parent *cobra.Command, pair func(*cobra.Command) error) {
	parent.AddCommand(&cobra.Command{Use: "check", Short: "Check stored bot authentication and polling prerequisites without consuming messages", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		binding, err := telegram.Load(root, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		store, err := secrets.Open()
		if err != nil {
			return err
		}
		token, err := store.Get(cmd.Context(), binding.TokenRef)
		if err != nil {
			return err
		}
		client, err := telegram.NewClient(binding.APIBase, token, c.Limits)
		if err != nil {
			return err
		}
		defer client.Close()
		bot, err := client.Identity(cmd.Context())
		if err != nil {
			return err
		}
		if bot.ID != binding.Bot.ID {
			return errors.New("stored token does not identify the paired bot")
		}
		if err = client.CheckPolling(cmd.Context()); err != nil {
			return err
		}
		return output(cmd, map[string]any{"bot_username": bot.Username, "identity_verified": true, "paired": binding.UserID > 0, "polling_prerequisites": true, "messages_consumed": false})
	}})
	parent.AddCommand(&cobra.Command{Use: "supervise", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return chatsupervisor.Run(cmd.Context(), root, c)
	}})
	for _, operation := range []string{"enable", "start", "stop", "restart", "disable", "remove", "render"} {
		var atLogin bool
		child := &cobra.Command{Use: operation, Short: "Manage persistent chat: " + operation, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			root, err := o.path()
			if err != nil {
				return err
			}
			c, err := config.Load(root)
			if err != nil {
				if operation != "enable" || !errors.Is(err, os.ErrNotExist) {
					return err
				}
				// Pairing retains its existing setup path. Defaults only bound the
				// private lifecycle lock until that path has persisted configuration.
				c = config.Defaults()
			}
			if operation == "render" {
				definition, err := service.RenderFor(root, service.Chat, atLogin)
				if err != nil {
					return err
				}
				if o.json {
					return output(cmd, definition)
				}
				_, err = fmt.Fprint(cmd.OutOrStdout(), definition.Body)
				return err
			}
			lock, err := telegramLifecycleLock(cmd.Context(), root, c)
			if err != nil {
				return errors.New("another Telegram lifecycle change is still in progress")
			}
			defer lock.Close()
			if err = cmd.Context().Err(); err != nil {
				return err
			}
			if operation == "enable" {
				binding, e := telegram.Load(root, c.Limits.MaxArtifactBytes)
				if e != nil || binding.UserID == 0 {
					if err = cmd.Context().Err(); err != nil {
						return err
					}
					if err = pair(cmd); err != nil {
						return err
					}
					if c, err = config.Load(root); err != nil {
						return err
					}
				}
			}
			var prior telegramProcesses
			if operation == "stop" || operation == "disable" || operation == "restart" || operation == "remove" {
				definition, registered, e := verifiedTelegramRegistration(root)
				if e != nil {
					return errors.New("the Telegram service registration could not be verified")
				}
				ownership, e := telegramOwnership(root, c)
				if e != nil {
					return errors.New("existing Telegram process ownership could not be verified")
				}
				prior = ownership
				if err = cmd.Context().Err(); err != nil {
					return err
				}
				if e = service.SetEnabled(root, false); e != nil {
					return errors.New("the Telegram stop intent could not be saved")
				}
				if e = stopTelegram(cmd.Context(), root, c, registered, ownership); e != nil {
					return errors.New("the Telegram stop could not be confirmed; explicit stop remains saved")
				}
				if operation == "remove" {
					if !registered {
						return errors.New("the Telegram service registration is not installed")
					}
					if err = cmd.Context().Err(); err != nil {
						return err
					}
					if _, e = service.RemoveFor(cmd.Context(), root, service.Chat, telegramLifecycleBudget(c)); e != nil {
						return errors.New("the Telegram service could not be removed")
					}
					return output(cmd, map[string]any{"removed": definition.Path, "data_preserved": true})
				}
				if operation != "restart" {
					return output(cmd, map[string]any{"enabled": false, "explicit_stop_saved": true})
				}
			}
			if operation == "enable" {
				if _, supervising, e := chatsupervisor.Read(root, c.Limits); e != nil {
					return e
				} else if !supervising {
					lock, e := telegram.Lock(cmd.Context(), root, c.Limits)
					if e != nil {
						return errors.New("기존 터미널의 Telegram 수신기를 먼저 중지한 뒤 telegram enable을 실행해 주세요")
					}
					_ = lock.Close()
				}
				if err = cmd.Context().Err(); err != nil {
					return err
				}
				if _, err = service.InstallFor(root, service.Chat, atLogin); err != nil {
					return err
				}
			}
			if _, err = service.ReadFor(root, service.Chat); err != nil {
				return fmt.Errorf("install the supervisor with chunsu telegram enable: %w", err)
			}
			if _, _, err = verifiedTelegramRegistration(root); err != nil {
				return errors.New("the Telegram service registration could not be verified")
			}
			if operation != "restart" {
				prior, err = telegramOwnership(root, c)
				if err != nil {
					return errors.New("existing Telegram process ownership could not be verified")
				}
			}
			fresh := operation == "restart"
			if err = cmd.Context().Err(); err != nil {
				return err
			}
			if err = service.SetEnabled(root, true); err != nil {
				return errors.New("the Telegram start intent could not be saved")
			}
			result, err := startTelegram(cmd.Context(), root, c, prior, fresh)
			if err != nil {
				return errors.New("the Telegram supervisor did not become ready; enabled intent remains saved")
			}
			return output(cmd, map[string]any{"enabled": true, "service": result.Label, "ready": true, "next": "chunsu telegram status", "diagnostics": "chunsu errors", "definition_root": filepath.Join(root, service.ChatDirectory)})
		}}
		if operation == "enable" || operation == "render" {
			child.Flags().BoolVar(&atLogin, "at-login", true, "Start at user login; on Linux a persistent user manager is required after logout/reboot")
		}
		parent.AddCommand(child)
	}
}
