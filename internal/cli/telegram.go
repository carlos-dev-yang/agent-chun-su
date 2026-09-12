package cli

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/secrets"
	"chunsu/internal/telegram"
	"github.com/spf13/cobra"
)

func (o *options) telegram() *cobra.Command {
	var tokenFile, apiBase string
	var pairingTimeout time.Duration
	cmd := &cobra.Command{Use: "telegram", Short: "Pair a private Telegram bot and chat through the configured executor", Args: cobra.NoArgs}
	cmd.Flags().StringVar(&tokenFile, "token-file", "", "Read a private local token file instead of masked terminal input; first setup only")
	cmd.Flags().StringVar(&apiBase, "api-base", telegram.DefaultAPIBase, "Telegram API base; first setup only")
	cmd.Flags().DurationVar(&pairingTimeout, "pair-timeout", time.Duration(telegram.PairSeconds)*time.Second, "Time to enter the local pairing code in the bot DM")
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Inspect saved Telegram binding without reading its token", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, e := o.path()
		if e != nil {
			return e
		}
		c, e := config.Load(root)
		if e != nil {
			return e
		}
		b, e := telegram.Load(root, c.Limits.MaxArtifactBytes)
		if errors.Is(e, os.ErrNotExist) {
			return output(cmd, map[string]any{"configured": false, "next": "chunsu telegram"})
		}
		if e != nil {
			return e
		}
		offset, e := telegram.Offset(root, c.Limits.MaxArtifactBytes)
		if e != nil {
			return e
		}
		return output(cmd, map[string]any{"configured": true, "bot_username": b.Bot.Username, "paired": b.UserID > 0, "next_update": offset, "runtime": "not inferred; inspect the running telegram terminal"})
	}})
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if pairingTimeout <= 0 {
			return errors.New("pair timeout must be positive")
		}
		root, e := o.path()
		if e != nil {
			return e
		}
		c, e := config.Load(root)
		if errors.Is(e, os.ErrNotExist) {
			setup := o.setup()
			setup.SetContext(cmd.Context())
			setup.SetOut(cmd.OutOrStdout())
			if e = setup.RunE(setup, nil); e != nil {
				return e
			}
			c, e = config.Load(root)
		}
		if e != nil {
			return e
		}
		selected := c.ExecutorFor(config.RoleReception)
		if selected.Kind != "codex" || selected.Path == "" || selected.Model == "" {
			return errors.New("먼저 대화 실행기의 kind/path/model을 설정해 주세요. chunsu config route reception으로 확인할 수 있습니다")
		}
		// Check before polling so incompatible installations do not consume DMs.
		compatibility := executor.Inspect(cmd.Context(), config.RoleReception, root, selected, c.Limits)
		if compatibility.Status != "prerequisites_match" {
			return fmt.Errorf("Telegram 대화 실행기 확인 실패: %s", compatibility.Detail)
		}
		lock, e := telegram.Lock(cmd.Context(), root, c.Limits)
		if e != nil {
			return e
		}
		defer lock.Close()
		keychain, e := secrets.Open()
		if e != nil {
			return e
		}
		binding, e := telegram.Load(root, c.Limits.MaxArtifactBytes)
		if errors.Is(e, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "춘수 전용 또는 다른 프로그램이 사용하지 않는 봇을 준비해 주세요. Telegram @BotFather에서 /newbot으로 만들 수 있습니다.")
			var token string
			if tokenFile != "" {
				p, e := filepath.Abs(tokenFile)
				if e != nil {
					return e
				}
				info, e := os.Lstat(p)
				if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
					return errors.New("token file must be a private regular file (0600)")
				}
				b, e := files.Read(filepath.Dir(p), filepath.Base(p), secrets.MaxSecretBytes)
				if e != nil {
					return errors.New("cannot read the local token file")
				}
				token = strings.TrimSpace(string(b))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), "BotFather 토큰을 입력하세요 (화면에 표시되지 않음): ")
				token, e = platform.ReadSecret(cmd.Context(), secrets.MaxSecretBytes)
				fmt.Fprintln(cmd.OutOrStdout())
				if e != nil {
					return e
				}
				token = strings.TrimSpace(token)
			}
			client, e := telegram.NewClient(apiBase, token, c.Limits)
			if e != nil {
				return e
			}
			defer client.Close()
			bot, e := client.Identity(cmd.Context())
			if e != nil {
				return e
			}
			if e = client.CheckPolling(cmd.Context()); e != nil {
				return e
			}
			binding = telegram.Binding{Version: telegram.Version, Bot: bot, TokenRef: files.ID(), APIBase: apiBase}
			if e = keychain.Set(cmd.Context(), binding.TokenRef, token); e != nil {
				return e
			}
			if e = telegram.Save(root, binding, false); e != nil {
				// Publishing can succeed before directory sync fails. Never
				// delete a credential that a visible binding may already use.
				published, readErr := telegram.Load(root, c.Limits.MaxArtifactBytes)
				if readErr == nil && published.TokenRef == binding.TokenRef {
					return errors.New("봇 연결 파일은 보이지만 저장 완료를 확인하지 못했습니다. 상태를 확인한 뒤 다시 실행해 주세요")
				}
				if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
					return errors.New("연결 저장 결과가 불명확합니다. Keychain 항목을 보존했으니 로컬 연결 상태를 확인해 주세요")
				}
				cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Limits.LockWaitSeconds)*time.Second)
				defer cancel()
				cleanup := keychain.Delete(cleanupCtx, binding.TokenRef)
				return errors.Join(e, cleanup)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "봇 신원을 확인하고 토큰을 Keychain에 저장했습니다.")
		} else if e != nil {
			return e
		} else if tokenFile != "" || cmd.Flags().Changed("api-base") {
			return errors.New("기존 연결을 보존했습니다. 저장된 봇은 추가 token/api-base 인자 없이 실행해 주세요")
		}
		token, e := keychain.Get(cmd.Context(), binding.TokenRef)
		if e != nil {
			return e
		}
		client, e := telegram.NewClient(binding.APIBase, token, c.Limits)
		if e != nil {
			return e
		}
		defer client.Close()
		bot, e := client.Identity(cmd.Context())
		if e != nil {
			return e
		}
		if bot.ID != binding.Bot.ID {
			return errors.New("stored token no longer identifies the paired bot")
		}
		if e = client.CheckPolling(cmd.Context()); e != nil {
			return e
		}
		if binding.UserID == 0 {
			binding, e = pairTelegram(cmd, root, c, binding, client, pairingTimeout)
			if e != nil {
				return e
			}
		}
		stop, e := startSetupHost(cmd.Context(), cmd, root, c)
		if e != nil {
			return e
		}
		defer stop()
		fmt.Fprintln(cmd.OutOrStdout(), "Telegram @"+binding.Bot.Username+" 에서 대화할 수 있습니다. 본인 DM만 처리합니다. 종료: 이 터미널에서 Ctrl-C.")
		return serveTelegram(cmd, root, c, binding, client)
	}
	return cmd
}

func pairTelegram(cmd *cobra.Command, root string, c config.Config, b telegram.Binding, client *telegram.Client, timeout time.Duration) (telegram.Binding, error) {
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	code := files.ID()
	fmt.Fprintf(cmd.OutOrStdout(), "Telegram @%s 의 개인 대화에서 아래 문장을 보내세요. 이 코드는 로컬 연결 확인용입니다.\n/start %s\n", b.Bot.Username, code)
	offset, e := telegram.Offset(root, c.Limits.MaxArtifactBytes)
	if e != nil {
		return b, e
	}
	for {
		updates, e := client.Updates(ctx, offset)
		if e != nil {
			return b, e
		}
		for _, u := range updates {
			if u.ID < offset {
				continue
			}
			if e = telegram.Advance(root, u.ID); e != nil {
				return b, e
			}
			offset = u.ID + 1
			if !u.Message.PrivateText() {
				continue
			}
			if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(u.Message.Text)), []byte("/start "+code)) != 1 {
				continue
			}
			b.UserID = u.Message.From.ID
			b.ChatID = u.Message.Chat.ID
			if e = telegram.Save(root, b, true); e != nil {
				return b, e
			}
			receipt := telegram.Receipt{UpdateID: u.ID, State: "pairing_saved"}
			if e = telegram.Record(root, receipt, false); e != nil {
				return b, e
			}
			receipt.MessageIDs, e = client.Send(ctx, b.ChatID, "춘수와 연결되었습니다. 이 개인 대화의 요청만 처리합니다. 대화는 설정된 AI 실행기로 전달됩니다. 토큰·비밀번호는 보내지 마세요. /help로 사용법을 확인할 수 있습니다.")
			if e != nil {
				receipt.State = "pair_reply_unconfirmed"
			} else {
				receipt.State = "paired"
			}
			saveErr := telegram.Record(root, receipt, true)
			if e != nil || saveErr != nil {
				return b, errors.Join(e, saveErr)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "본인 DM 페어링과 확인 답장을 완료했습니다.")
			return b, nil
		}
	}
}
