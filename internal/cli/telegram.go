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
	"chunsu/internal/errorreport"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/secrets"
	"chunsu/internal/telegram"
	"github.com/spf13/cobra"
)

func (o *options) telegram() *cobra.Command {
	var tokenFile, apiBase string
	var pairingTimeout time.Duration
	var pairedOnly, pairOnly bool
	cmd := &cobra.Command{Use: "telegram", Short: "Pair a private Telegram bot and chat through the configured executor", Args: cobra.NoArgs}
	cmd.PersistentFlags().StringVar(&tokenFile, "token-file", "", "Read a private local token file instead of masked terminal input; first setup only")
	cmd.PersistentFlags().StringVar(&apiBase, "api-base", telegram.DefaultAPIBase, "Telegram API base; first setup only")
	cmd.PersistentFlags().DurationVar(&pairingTimeout, "pair-timeout", time.Duration(telegram.PairSeconds)*time.Second, "Time to enter the local pairing code in the bot DM")
	cmd.Flags().BoolVar(&pairedOnly, "paired-only", false, "Require a previously paired bot; never prompt")
	_ = cmd.Flags().MarkHidden("paired-only")
	cmd.AddCommand(o.telegramStatus())

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
		// AI compatibility is checked per turn; fixed commands remain available.
		lock, e := telegram.Lock(cmd.Context(), root, c.Limits)
		if e != nil {
			return e
		}
		defer lock.Close()
		var keychain secrets.Store
		binding, e := telegram.Load(root, c.Limits.MaxArtifactBytes)
		if errors.Is(e, os.ErrNotExist) {
			keychain, e = secrets.Open()
			if e != nil {
				return e
			}
			if pairedOnly {
				return errors.New("pair Telegram locally before enabling its service")
			}
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
		if binding.UserID > 0 {
			if pairOnly {
				return output(cmd, map[string]any{"paired": true, "bot_username": binding.Bot.Username})
			}
			return runPairedTelegram(cmd, root, c, binding)
		}
		if pairedOnly {
			return errors.New("Telegram pairing is incomplete")
		}
		if keychain == nil {
			keychain, e = secrets.Open()
			if e != nil {
				return e
			}
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
		if pairOnly {
			return output(cmd, map[string]any{"paired": true, "bot_username": binding.Bot.Username})
		}
		return runPairedTelegram(cmd, root, c, binding)
	}
	pair := &cobra.Command{Use: "pair", Short: "Pair the bot locally without keeping a terminal receiver open", Args: cobra.NoArgs, RunE: func(child *cobra.Command, args []string) error {
		pairOnly = true
		defer func() { pairOnly = false }()
		return cmd.RunE(child, args)
	}}
	cmd.AddCommand(pair)
	o.addTelegramServiceCommands(cmd, func(child *cobra.Command) error { return pair.RunE(child, nil) })
	return cmd
}

func runPairedTelegram(cmd *cobra.Command, root string, c config.Config, binding telegram.Binding) error {
	reports := errorreport.New(root, c.Limits)
	failures := 0
	identity, err := platform.Identify(os.Getpid())
	if err != nil {
		return err
	}
	health := telegram.Health{Identity: identity, StartedAt: time.Now().UTC(), State: "connecting", Poll: "connecting"}
	writeHealth := func() { health.UnpersistedErrors = reports.Unpersisted(); _ = telegram.WriteHealth(root, health) }
	defer func() { health.State = "stopped"; writeHealth() }()
	heartbeat := time.NewTicker(telegram.HealthInterval(c.Limits))
	defer heartbeat.Stop()
	for cmd.Context().Err() == nil {
		health.State = "connecting"
		writeHealth()
		err := func() error {
			keychain, err := secrets.Open()
			if err != nil {
				return err
			}
			token, err := keychain.Get(cmd.Context(), binding.TokenRef)
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
				return errors.New("stored token no longer identifies the paired bot")
			}
			if err = client.CheckPolling(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Telegram @"+binding.Bot.Username+" 에서 대화할 수 있습니다. 접수 문구와 고정 관리 명령은 AI 준비 상태와 별개로 동작합니다.")
			return serveTelegram(cmd, root, c, binding, client)
		}()
		if cmd.Context().Err() != nil {
			return nil
		}
		code := "startup_failed"
		if errors.Is(err, secrets.ErrMissing) || errors.Is(err, secrets.ErrUnavailable) {
			code = "secret_unavailable"
		}
		if telegram.ErrorCode(err) != "poll_failed" {
			code = telegram.ErrorCode(err)
		}
		id, _ := reports.Record(cmd.Context(), code, errorreport.Correlation{})
		if failures == 0 {
			fmt.Fprintln(cmd.ErrOrStderr(), "채팅 연결을 재시도합니다. chunsu errors로 확인하세요. 오류 ID:", id)
		}
		failures++
		health.State = "retry_wait"
		health.Poll = code
		writeHealth()
		timer := time.NewTimer(telegram.RetryDelay(err, failures, c.Limits))
		waiting := true
		for waiting {
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return nil
			case <-timer.C:
				waiting = false
			case <-heartbeat.C:
				_ = reports.Flush(cmd.Context())
				writeHealth()
			}
		}
		if current, e := config.Load(root); e == nil {
			c = current
		}
		if current, e := telegram.Load(root, c.Limits.MaxArtifactBytes); e == nil && current.UserID > 0 {
			binding = current
		}
	}
	return nil
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
