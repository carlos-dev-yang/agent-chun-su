package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/reception"
	"chunsu/internal/telegram"
	"github.com/spf13/cobra"
)

type telegramTurnResult struct {
	history []conversation.Event
	err     error
	fatal   bool
}

func telegramFailureMessage(err error) string {
	var compatibility *executor.CompatibilityError
	if errors.As(err, &compatibility) {
		return "AI 실행기 호환성 문제로 답변을 생성하지 못했습니다. " + compatibility.Error() + " 호스트 터미널의 chunsu doctor로 확인하고 지원되는 실행기를 설정하거나 춘수를 업데이트한 뒤 다시 실행해 주세요."
	}
	return "요청을 완료하지 못했습니다. 설치된 춘수 터미널에서 원인을 확인해 주세요."
}

func telegramTurn(ctx context.Context, root string, c config.Config, sessionID string, history []conversation.Event, u telegram.Update, emit func(string) error) telegramTurnResult {
	session := reception.New(root, c, reception.Telegram)
	session.ID = "telegram-" + sessionID
	session.History = history
	// Each remote turn starts without remembered reference authority. A fresh
	// list_jobs result or an explicit user ID admits a selected existing job.
	forward := func(event reception.Event) error {
		switch event.Kind {
		case "reply":
			return emit(event.Text)
		case "action":
			return emit("[호스트 처리 결과] " + event.Action + ": " + event.Text)
		}
		return nil
	}
	err := session.Turn(ctx, u.Message.Text, reception.Host{Root: root, Config: c}, nil, forward)
	return telegramTurnResult{history: session.History, err: err, fatal: errors.Is(err, reception.ErrUncertain)}
}

func serveTelegram(cmd *cobra.Command, root string, c config.Config, b telegram.Binding, client *telegram.Client) error {
	ctx, cancelAll := context.WithCancel(cmd.Context())
	defer cancelAll()
	offset, e := telegram.Offset(root, c.Limits.MaxArtifactBytes)
	if e != nil {
		return e
	}
	type incoming struct {
		update telegram.Update
		ack    chan struct{}
		err    error
	}
	incomingCh := make(chan incoming)
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		for {
			updates, e := client.Updates(ctx, offset)
			if e != nil {
				select {
				case incomingCh <- incoming{err: e}:
				case <-ctx.Done():
				}
				return
			}
			for _, u := range updates {
				if u.ID < offset {
					continue
				}
				ack := make(chan struct{})
				select {
				case incomingCh <- incoming{update: u, ack: ack}:
				case <-ctx.Done():
					return
				}
				select {
				case <-ack:
					offset = u.ID + 1
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	var activeCancel context.CancelFunc
	var done chan telegramTurnResult
	history := []conversation.Event{}
	session := files.ID()
	defer func() {
		cancelAll()
		if activeCancel != nil {
			activeCancel()
			<-done
		}
		<-pollDone
	}()
	help := "춘수에게 할 일을 자연스럽게 말씀해 주세요. 일반 대화·초안 작성, 실행 상태, 작업 목록, 서비스 설정 자료를 지원합니다. Gmail 인증과 보고서 본문은 호스트의 로컬 화면에서 진행합니다. /cancel 현재 답변 중단, /reset 새 대화, /help 사용법."
	sendRecorded := func(ctx context.Context, receipt *telegram.Receipt, text string) error {
		receipt.State = "reply_sending"
		receipt.ReplyDigest = files.Digest([]byte(text))
		if e := telegram.Record(root, *receipt, true); e != nil {
			return e
		}
		ids, e := client.Send(ctx, b.ChatID, text)
		receipt.MessageIDs = append(receipt.MessageIDs, ids...)
		if e != nil {
			receipt.State = "reply_unconfirmed"
		} else {
			receipt.State = "reply_sent"
		}
		saveErr := telegram.Record(root, *receipt, true)
		return errors.Join(e, saveErr)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case result := <-done:
			activeCancel()
			activeCancel = nil
			done = nil
			history = result.history
			if result.fatal {
				return result.err
			}
			if result.err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Telegram 요청 처리:", result.err)
			}
		case event := <-incomingCh:
			if event.err != nil {
				return event.err
			}
			u := event.update
			if u.ID < 0 {
				return errors.New("invalid Telegram update")
			}
			permitted := u.Message.PrivateText() && u.Message.From.ID == b.UserID && u.Message.Chat.ID == b.ChatID
			if !permitted {
				if e = telegram.Advance(root, u.ID); e != nil {
					return e
				}
				close(event.ack)
				continue
			}
			receipt := telegram.Receipt{UpdateID: u.ID, State: "claimed", InputDigest: files.Digest([]byte(u.Message.Text))}
			if e = telegram.Record(root, receipt, false); e != nil {
				if !errors.Is(e, os.ErrExist) {
					return e
				}
				// A durable claim, including an interrupted one, is never replayed.
				if e = telegram.Advance(root, u.ID); e != nil {
					return e
				}
				close(event.ack)
				continue
			}
			if e = telegram.Advance(root, u.ID); e != nil {
				return e
			}
			close(event.ack)
			text := strings.TrimSpace(u.Message.Text)
			switch text {
			case "/cancel", "취소", "/reset", "/새대화":
				if activeCancel != nil {
					activeCancel()
					result := <-done
					activeCancel = nil
					done = nil
					history = result.history
					if result.fatal && !errors.Is(result.err, context.Canceled) {
						return result.err
					}
				}
				response := "현재 답변을 중단했습니다. 이미 완료된 호스트 작업은 취소되지 않습니다."
				if text == "/reset" || text == "/새대화" {
					history = nil
					session = files.ID()
					response = "새 대화를 시작했습니다."
				} else {
					history = append(history, conversation.Event{Role: "host", Content: "사용자가 진행 중 요청을 취소했습니다. 이전 미완료 작업을 자동 재실행하지 마세요."})
				}
				if e = sendRecorded(ctx, &receipt, response); e != nil {
					return e
				}
			case "/help", "/start":
				if e = sendRecorded(ctx, &receipt, help); e != nil {
					return e
				}
			default:
				if strings.HasPrefix(text, "/start ") {
					if e = sendRecorded(ctx, &receipt, "이미 본인 DM과 연결되어 있습니다. /help로 사용법을 확인하세요."); e != nil {
						return e
					}
					continue
				}
				if int64(len(text)) > c.Limits.MaxSourceBytes {
					if e = sendRecorded(ctx, &receipt, "입력이 너무 깁니다. 내용을 나누어 보내 주세요."); e != nil {
						return e
					}
					continue
				}
				if activeCancel != nil {
					if e = sendRecorded(ctx, &receipt, "앞선 요청을 처리 중입니다. 답변을 기다리거나 /cancel 후 새 요청을 보내 주세요."); e != nil {
						return e
					}
					continue
				}
				workCtx, stop := context.WithCancel(ctx)
				activeCancel = stop
				done = make(chan telegramTurnResult, 1)
				go func(done chan telegramTurnResult, receipt telegram.Receipt) {
					emit := func(text string) error { return sendRecorded(workCtx, &receipt, text) }
					result := telegramTurn(workCtx, root, c, session, history, u, emit)
					if result.err != nil && !result.fatal && workCtx.Err() == nil {
						if e := emit(telegramFailureMessage(result.err)); e != nil {
							result.err = e
							result.fatal = true
						}
					}
					switch {
					case result.fatal:
						receipt.State = "uncertain"
					case workCtx.Err() != nil:
						receipt.State = "cancelled"
					case result.err != nil:
						receipt.State = "failed"
					default:
						receipt.State = "completed"
					}
					if e := telegram.Record(root, receipt, true); e != nil {
						result.err = e
						result.fatal = true
					}
					done <- result
				}(done, receipt)
			}
		}
	}
}
