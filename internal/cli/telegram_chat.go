package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/telegram"
	"github.com/spf13/cobra"
)

func telegramCapabilities() []conversation.Capability {
	allowed := []conversation.Capability{}
	for _, c := range conversation.Capabilities() {
		switch c.Name {
		case conversation.None, conversation.RuntimeStatus, conversation.ReadGuide, conversation.InstallGuide, conversation.ListJobs:
			allowed = append(allowed, c)
		}
	}
	return allowed
}
func allowedTelegramAction(name string) bool {
	for _, c := range telegramCapabilities() {
		if c.Name == name {
			return true
		}
	}
	return false
}

type telegramTurnResult struct {
	history []conversation.Event
	err     error
	fatal   bool
}

func telegramTurn(ctx context.Context, root string, c config.Config, session string, history []conversation.Event, u telegram.Update, emit func(string) error) telegramTurnResult {
	history = append(history, conversation.Event{Role: "user", Content: u.Message.Text})
	result := telegramTurnResult{}
	fail := func(e error, fatal bool) telegramTurnResult {
		return telegramTurnResult{history: history, err: e, fatal: fatal}
	}
	schema, e := conversation.SchemaFor(telegramCapabilities())
	if e != nil {
		return fail(e, true)
	}
	skill, e := conversation.Skill()
	if e != nil {
		return fail(e, true)
	}
	d := &setupDialogue{ctx: ctx, root: root, config: c, out: io.Discard, noBrowser: true}
	for step := 0; step < min(conversation.MaxActionsPerTurn, c.Limits.MaxToolCalls); step++ {
		prompt, e := conversation.PromptFor(history, c.Limits.MaxSourceBytes, telegramCapabilities(), "telegram_paired_private_dm")
		if e != nil {
			return fail(errors.New("대화가 길어졌습니다. /reset으로 새 대화를 시작해 주세요."), false)
		}
		dir := filepath.Join(root, "chat", "telegram-"+session, files.ID())
		generated, e := executor.Converse(ctx, root, dir, c.Executor, c.Limits, prompt, schema, skill)
		if e != nil {
			return fail(e, generated.Outcome == "orphaned")
		}
		reply, e := conversation.Decode(generated.Final)
		if e != nil || !allowedTelegramAction(reply.Action.Name) {
			return fail(errors.New("이 대화에서 허용되지 않은 작업 제안을 거부했습니다."), false)
		}
		if e = emit(reply.Message); e != nil {
			return fail(e, true)
		}
		encoded, _ := json.Marshal(reply)
		history = append(history, conversation.Event{Role: "assistant", Content: string(encoded)})
		if reply.Action.Name == conversation.None {
			return telegramTurnResult{history: history}
		}
		if e = ctx.Err(); e != nil {
			return fail(e, false)
		}
		intent, _ := json.Marshal(map[string]any{"action": reply.Action, "state": "requested"})
		if e = files.Write(dir, "action.json", intent, false); e != nil {
			return fail(e, true)
		}
		// Remote projection is checked immediately before dispatch as well as after decoding.
		if !allowedTelegramAction(reply.Action.Name) {
			return fail(errors.New("remote action denied"), true)
		}
		outcome, actionErr := d.chatAction(reply.Action, u.Message.Text, map[string]bool{})
		if actionErr != nil {
			outcome = chatHostResult{Status: "failed", Detail: "호스트 작업을 완료하지 못했습니다. 로컬 상태를 확인해 주세요. 자동 재시도하지 마세요."}
		}
		b, _ := json.Marshal(outcome)
		audit, _ := json.Marshal(map[string]any{"action": reply.Action, "state": outcome.Status, "result_digest": files.Digest(b)})
		if e = files.Write(dir, "action.json", audit, true); e != nil {
			return fail(e, true)
		}
		history = append(history, conversation.Event{Role: "host", Content: string(b)})
		if e = emit("[호스트 처리 결과] " + reply.Action.Name + ": " + outcome.Status); e != nil {
			return fail(e, true)
		}
		if actionErr != nil {
			return fail(errors.New("작업이 완료되지 않았습니다. Mac의 춘수에서 상태를 확인해 주세요."), false)
		}
	}
	result.history = history
	result.err = errors.New("이번 요청의 처리 한도에 도달했습니다. 확인된 결과를 바탕으로 이어서 요청해 주세요.")
	return result
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
	help := "춘수에게 할 일을 자연스럽게 말씀해 주세요. 일반 대화·초안 작성, 실행 상태, 작업 목록, 서비스 설정 자료를 지원합니다. Gmail 인증과 보고서 본문은 Mac에서 진행합니다. /cancel 현재 답변 중단, /reset 새 대화, /help 사용법."
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
						if e := emit("요청을 완료하지 못했습니다. Mac의 춘수 터미널에서 원인을 확인해 주세요. 대화가 길어졌다면 /reset으로 새로 시작할 수 있습니다."); e != nil {
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
