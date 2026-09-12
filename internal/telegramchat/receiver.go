// Package telegramchat connects transport to reception independently of CLI UI.
package telegramchat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/reception"
	"chunsu/internal/telegram"
)

const Acknowledgment = "접수했습니다."

type Receiver struct {
	Root    string
	Config  config.Config
	Binding telegram.Binding
	Client  *telegram.Client
	Errors  *errorreport.Recorder
}
type turnResult struct {
	history             []conversation.Event
	err                 error
	uncertain, panicked bool
}

func FailureMessage(err error) string {
	var compatibility *executor.CompatibilityError
	if errors.As(err, &compatibility) {
		return "AI 실행기 호환성 문제로 답변을 생성하지 못했습니다. " + compatibility.Error() + " 호스트의 chunsu doctor로 확인해 주세요."
	}
	return "요청을 완료하지 못했습니다. /errors로 원인과 복구 안내를 확인할 수 있습니다. /status와 /help는 계속 사용할 수 있습니다."
}
func (r Receiver) record(ctx context.Context, code string, id int64, session string) string {
	errorID, _ := r.Errors.Record(ctx, code, errorreport.Correlation{UpdateID: id, SessionID: session})
	return errorID
}
func (r Receiver) send(ctx context.Context, receipt *telegram.Receipt, text string, ack bool) error {
	if ack {
		receipt.Acknowledgment = "sending"
	} else {
		receipt.State = "reply_sending"
		receipt.ReplyDigest = files.Digest([]byte(text))
	}
	if err := telegram.Record(r.Root, *receipt, true); err != nil {
		r.record(ctx, "receipt_failed", receipt.UpdateID, receipt.SessionID)
		return err
	}
	ids, err := r.Client.Send(ctx, r.Binding.ChatID, text)
	if ack {
		receipt.AckMessageIDs = ids
		receipt.Acknowledgment = "sent"
		if err != nil {
			receipt.Acknowledgment = "unconfirmed"
		}
	} else {
		receipt.MessageIDs = append(receipt.MessageIDs, ids...)
		receipt.State = "reply_sent"
		if err != nil {
			receipt.State = "reply_unconfirmed"
		}
	}
	if err != nil {
		receipt.ErrorID = r.record(ctx, "send_unconfirmed", receipt.UpdateID, receipt.SessionID)
	}
	saveErr := telegram.Record(r.Root, *receipt, true)
	if saveErr != nil {
		r.record(ctx, "receipt_failed", receipt.UpdateID, receipt.SessionID)
	}
	return errors.Join(err, saveErr)
}
func (r Receiver) turn(ctx, notifyCtx context.Context, sessionID string, history []conversation.Event, update telegram.Update, receipt telegram.Receipt) (result turnResult) {
	// Never retain the panic value: it can contain private conversation data.
	defer func() {
		if recover() != nil {
			result.err = errors.New("internal reception failure")
			result.uncertain = true
			result.panicked = true
		}
		deadline := errors.Is(ctx.Err(), context.DeadlineExceeded)
		if deadline {
			result.err = context.DeadlineExceeded
		}
		if result.err != nil && (ctx.Err() == nil || deadline) {
			code := reception.ErrorCode(result.err, r.Config)
			if result.uncertain {
				code = "execution_uncertain"
			}
			if result.panicked {
				code = "turn_panicked"
			}
			receipt.ErrorID = r.record(notifyCtx, code, update.ID, sessionID)
			if receipt.State != "reply_unconfirmed" {
				_ = r.send(notifyCtx, &receipt, FailureMessage(result.err)+"\n오류 ID: "+receipt.ErrorID, false)
			}
		}
		switch {
		case receipt.State == "reply_unconfirmed":
		case result.uncertain:
			receipt.State = "uncertain"
		case deadline:
			receipt.State = "failed"
		case ctx.Err() != nil:
			receipt.State = "cancelled"
		case result.err != nil:
			receipt.State = "failed"
		default:
			receipt.State = "completed"
		}
		if err := telegram.Record(r.Root, receipt, true); err != nil {
			r.record(context.Background(), "receipt_failed", update.ID, sessionID)
			result.uncertain = true
		}
	}()
	session := reception.New(r.Root, r.Config, reception.Telegram)
	session.ID = "telegram-" + sessionID
	session.History = history
	emit := func(event reception.Event) error {
		switch event.Kind {
		case "reply":
			return r.send(ctx, &receipt, event.Text, false)
		case "action":
			return r.send(ctx, &receipt, "[호스트 처리 결과] "+event.Action+": "+event.Text, false)
		}
		return nil
	}
	result.err = session.Turn(ctx, update.Message.Text, reception.Host{Root: r.Root, Config: r.Config}, nil, emit)
	result.uncertain = errors.Is(result.err, reception.ErrUncertain)
	result.history = session.History
	return result
}

type incoming struct {
	update  *telegram.Update
	err     error
	advance chan bool
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func (r Receiver) poll(ctx context.Context, offset int64, events chan<- incoming) {
	failures := 0
	for ctx.Err() == nil {
		updates, err := r.Client.Updates(ctx, offset)
		select {
		case events <- incoming{err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			failures++
			if !wait(ctx, telegram.RetryDelay(err, failures, r.Config.Limits)) {
				return
			}
			continue
		}
		failures = 0
		for _, update := range updates {
			if update.ID < offset {
				continue
			}
			advance := make(chan bool, 1)
			select {
			case events <- incoming{update: &update, advance: advance}:
			case <-ctx.Done():
				return
			}
			select {
			case ok := <-advance:
				if ok {
					offset = update.ID + 1
				} else {
					if !wait(ctx, telegram.RetryDelay(nil, 1, r.Config.Limits)) {
						return
					}
				}
			case <-ctx.Done():
				return
			}
			if offset <= update.ID {
				break
			}
		}
		if len(updates) == 0 && !wait(ctx, time.Duration(r.Config.Limits.PollSeconds)*time.Second) {
			return
		}
	}
}

func (r Receiver) Serve(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if r.Errors == nil {
		r.Errors = errorreport.New(r.Root, r.Config.Limits)
	}
	offset, err := telegram.Offset(r.Root, r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		r.record(ctx, "receipt_failed", 0, "")
		return err
	}
	identity, err := platform.Identify(os.Getpid())
	if err != nil {
		return err
	}
	health := telegram.Health{Identity: identity, StartedAt: time.Now().UTC(), State: "receiving", Poll: "connecting", SessionID: files.ID()}
	if err = Reconcile(ctx, r.Root, r.Config.Limits); err != nil {
		health.AIBlocked = true
		r.record(ctx, "recovery_blocked", 0, "")
	}
	if err = r.recoverReceipts(ctx); err != nil {
		r.record(ctx, "receipt_failed", 0, "")
	}
	events := make(chan incoming)
	pollDone := make(chan struct{})
	go func(receiver Receiver) { defer close(pollDone); receiver.poll(ctx, offset, events) }(r)
	ticker := time.NewTicker(telegram.HealthInterval(r.Config.Limits))
	defer ticker.Stop()
	var activeCancel context.CancelFunc
	var done chan turnResult
	var history []conversation.Event
	resetPending := false
	defer func() {
		cancel()
		if activeCancel != nil {
			activeCancel()
			timer := time.NewTimer(time.Duration(r.Config.Limits.LockWaitSeconds+telegram.HTTPGraceSeconds) * time.Second)
			select {
			case <-done:
			case <-timer.C:
				r.record(context.Background(), "execution_uncertain", health.ActiveUpdate, health.SessionID)
			}
			timer.Stop()
		}
		<-pollDone
		health.State = "stopped"
		_ = telegram.WriteHealth(r.Root, health)
	}()
	writeHealth := func() {
		health.UnpersistedErrors = r.Errors.Unpersisted()
		if e := telegram.WriteHealth(r.Root, health); e != nil {
			r.record(ctx, "receipt_failed", 0, "")
		}
	}
	writeHealth()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = r.Errors.Flush(ctx)
			writeHealth()
		case result := <-done:
			activeCancel()
			activeCancel = nil
			done = nil
			history = result.history
			health.ActiveUpdate = 0
			if result.uncertain {
				if e := Reconcile(ctx, r.Root, r.Config.Limits); e != nil {
					health.AIBlocked = true
					r.record(ctx, "recovery_blocked", 0, "")
				}
				history = append(history, conversation.Event{Role: "host", Content: "이전 요청의 완료를 확인하지 못했습니다. 기존 작업을 자동 재실행하지 말고 사용자의 다음 요청을 기다리세요."})
			}
			if resetPending {
				history = nil
				health.SessionID = files.ID()
				resetPending = false
			}
			writeHealth()
		case event := <-events:
			if event.update == nil {
				if event.err != nil {
					// Re-open the secret store on authentication failures so a
					// host-side token repair takes effect without a manual restart.
					if telegram.ErrorCode(event.err) == "authentication_failed" {
						return event.err
					}
					health.Poll = telegram.ErrorCode(event.err)
					r.record(ctx, health.Poll, 0, "")
				} else {
					health.Poll = "connected"
					health.LastPollAt = time.Now().UTC()
				}
				writeHealth()
				continue
			}
			u := *event.update
			if !u.Message.PrivateText() || u.Message.From.ID != r.Binding.UserID || u.Message.Chat.ID != r.Binding.ChatID {
				e := telegram.Advance(r.Root, u.ID)
				if e != nil {
					r.record(ctx, "receipt_failed", 0, "")
				}
				event.advance <- e == nil
				continue
			}
			receipt := telegram.Receipt{UpdateID: u.ID, State: "claimed", InputDigest: files.Digest([]byte(u.Message.Text)), SessionID: health.SessionID}
			e := telegram.Record(r.Root, receipt, false)
			duplicate := errors.Is(e, os.ErrExist)
			if e != nil && !duplicate {
				r.record(ctx, "receipt_failed", u.ID, health.SessionID)
				event.advance <- false
				continue
			}
			if e = telegram.Advance(r.Root, u.ID); e != nil {
				r.record(ctx, "receipt_failed", u.ID, health.SessionID)
				event.advance <- false
				continue
			}
			event.advance <- true
			if duplicate {
				continue
			}
			health.LastMessageAt = time.Now().UTC()
			text := strings.TrimSpace(u.Message.Text)
			reply := func(text string) {
				if e := r.send(ctx, &receipt, text, false); e == nil {
					health.LastReplyAt = time.Now().UTC()
					receipt.State = "completed"
					if e = telegram.Record(r.Root, receipt, true); e != nil {
						r.record(ctx, "receipt_failed", u.ID, health.SessionID)
					}
				}
			}
			if text == "/cancel" || text == "취소" || text == "/reset" || text == "/새대화" {
				reset := text == "/reset" || text == "/새대화"
				if activeCancel != nil {
					activeCancel()
					resetPending = resetPending || reset
					reply("현재 답변 중단을 요청했습니다. 이미 수행한 작업은 유지되며, 중단 확인 전에는 새 AI 요청을 시작하지 않습니다.")
				} else if reset {
					if e := Reconcile(ctx, r.Root, r.Config.Limits); e == nil {
						health.AIBlocked = false
					}
					history = nil
					health.SessionID = files.ID()
					reply("새 대화를 시작했습니다.")
				} else {
					reply("현재 진행 중인 답변이 없습니다.")
				}
				continue
			}
			current, configErr := config.Load(r.Root)
			if configErr == nil {
				r.Config = current
			}
			host := reception.Host{Root: r.Root, Config: r.Config}
			response, handled, commandErr := host.Command(ctx, reception.Telegram, text)
			if handled {
				if commandErr != nil {
					receipt.ErrorID = r.record(ctx, "host_failed", u.ID, health.SessionID)
					response = "내부 작업을 완료하지 못했습니다. /errors로 확인해 주세요.\n오류 ID: " + receipt.ErrorID
				}
				reply(response)
				continue
			}
			if int64(len(text)) > r.Config.Limits.MaxSourceBytes {
				reply("입력이 너무 깁니다. 내용을 나누어 보내 주세요.")
				continue
			}
			if activeCancel != nil {
				reply("앞선 요청을 처리 중입니다. 답변을 기다리거나 /cancel 후 새 요청을 보내 주세요. 고정 관리 명령은 사용할 수 있습니다.")
				continue
			}
			if health.AIBlocked || configErr != nil {
				reply("AI 실행 복구가 필요합니다. /status와 /errors를 확인해 주세요. 고정 관리 명령은 계속 사용할 수 있습니다.")
				continue
			}
			if e = r.send(ctx, &receipt, Acknowledgment, true); e != nil {
				continue
			}
			workCtx, stop := context.WithTimeout(ctx, time.Duration(r.Config.Limits.TimeoutSeconds)*time.Second)
			activeCancel = stop
			done = make(chan turnResult, 1)
			health.ActiveUpdate = u.ID
			writeHealth()
			go func(receiver Receiver, result chan<- turnResult, session string, prior []conversation.Event) {
				result <- receiver.turn(workCtx, ctx, session, prior, u, receipt)
			}(r, done, health.SessionID, append([]conversation.Event(nil), history...))
		}
	}
}

// Reconcile touches only tool-free Telegram reception processes. Task workers
// and local terminal conversations have separate ownership and recovery paths.
func Reconcile(ctx context.Context, root string, limits config.Limits) error {
	entries, err := os.ReadDir(filepath.Join(root, "chat"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "telegram-") || !files.ValidID(strings.TrimPrefix(entry.Name(), "telegram-")) {
			continue
		}
		directory := filepath.Join("chat", entry.Name())
		attempts, err := os.ReadDir(filepath.Join(root, directory))
		if err != nil {
			return err
		}
		for _, attempt := range attempts {
			if !attempt.IsDir() || !files.ValidID(attempt.Name()) {
				continue
			}
			if err := executor.Reconcile(ctx, root, filepath.Join(directory, attempt.Name(), executor.ProcessFile), limits); err != nil {
				return err
			}
		}
	}
	return nil
}
func (r Receiver) recoverReceipts(ctx context.Context) error {
	entries, err := os.ReadDir(filepath.Join(telegram.Directory(r.Root), "receipts"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var id int64
		if _, e := fmt.Sscanf(entry.Name(), "%d.json", &id); e != nil || telegram.ReceiptName(id) != entry.Name() {
			continue
		}
		receipt, err := telegram.ReadReceipt(r.Root, id, r.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		switch receipt.State {
		case "claimed", "reply_sending", "reply_sent":
			receipt.ErrorID = r.record(ctx, "interrupted_request", id, receipt.SessionID)
			receipt.State = "interrupted"
			if err = telegram.Record(r.Root, receipt, true); err != nil {
				return err
			}
		}
	}
	return nil
}
