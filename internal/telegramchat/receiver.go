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

	"chunsu/internal/chatlanguage"
	"chunsu/internal/chatstyle"
	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/reception"
	"chunsu/internal/selfupdate"
	"chunsu/internal/telegram"
	"chunsu/internal/updateguard"
)

type Receiver struct {
	Root    string
	Config  config.Config
	Binding telegram.Binding
	Client  *telegram.Client
	Errors  *errorreport.Recorder
}
type turnResult struct {
	history                        []conversation.Event
	err                            error
	uncertain, panicked, cancelled bool
}
type controlNotice struct {
	receipt telegram.Receipt
	reset   bool
}
type commandRequest struct {
	receipt telegram.Receipt
	text    string
	config  config.Config
}
type commandResult struct {
	receipt  telegram.Receipt
	response string
	err      error
}

func (r Receiver) record(ctx context.Context, code string, id int64, session string) string {
	errorID, _ := r.Errors.Record(ctx, code, errorreport.Correlation{UpdateID: id, SessionID: session})
	return errorID
}
func (r Receiver) send(ctx context.Context, receipt *telegram.Receipt, text string) error {
	receipt.State = "reply_sending"
	receipt.ReplyDigest = files.Digest([]byte(text))
	if err := telegram.Record(r.Root, *receipt, true); err != nil {
		r.record(ctx, "receipt_failed", receipt.UpdateID, receipt.SessionID)
		return err
	}
	ids, err := r.Client.Send(ctx, r.Binding.ChatID, text)
	receipt.MessageIDs = append(receipt.MessageIDs, ids...)
	receipt.State = "reply_sent"
	if err != nil {
		receipt.State = "reply_unconfirmed"
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
func (r Receiver) acknowledge(ctx context.Context, receipt *telegram.Receipt, messageID int64) error {
	receipt.Acknowledgment = "sending"
	if err := telegram.Record(r.Root, *receipt, true); err != nil {
		r.record(ctx, "receipt_failed", receipt.UpdateID, receipt.SessionID)
		return err
	}
	err := r.Client.React(ctx, r.Binding.ChatID, messageID)
	receipt.Acknowledgment = "sent"
	if err != nil {
		receipt.Acknowledgment = "unconfirmed"
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
		result.cancelled = errors.Is(ctx.Err(), context.Canceled) && errors.Is(result.err, context.Canceled)
		if deadline {
			result.err = errors.Join(result.err, context.DeadlineExceeded)
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
				failure := result.err
				if result.uncertain {
					failure = errors.Join(reception.ErrUncertain, failure)
				}
				_ = r.send(notifyCtx, &receipt, reception.FailureMessage(failure)+"\nError ID: "+receipt.ErrorID)
			}
		}
		switch {
		case receipt.State == "reply_unconfirmed":
		case result.uncertain:
			receipt.State = "uncertain"
		case deadline:
			receipt.State = "failed"
		case result.cancelled:
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
	session.UpdateID = update.ID
	session.History = history
	emit := func(event reception.Event) error {
		if !event.UserVisible() {
			return nil
		}
		return r.send(ctx, &receipt, event.Text)
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
	commandCtx, cancelCommands := context.WithCancel(ctx)
	queueSize := r.Config.Limits.MaxMessages
	if queueSize < 1 {
		queueSize = 1
	}
	commands := make(chan commandRequest, queueSize)
	commandResults := make(chan commandResult, queueSize)
	commandsDone := make(chan struct{})
	go func() {
		defer close(commandsDone)
		for {
			select {
			case <-commandCtx.Done():
				return
			case request := <-commands:
				if commandCtx.Err() != nil {
					return
				}
				// Service control is bounded independently from the reception
				// loop so a stuck OS manager cannot stall polling or cancellation.
				operationCtx, stop := context.WithTimeout(commandCtx, telegram.StallTimeout(request.config.Limits))
				response, _, err := (reception.Host{Root: r.Root, Config: request.config}).Command(operationCtx, reception.Telegram, request.text)
				stop()
				select {
				case commandResults <- commandResult{receipt: request.receipt, response: response, err: err}:
				case <-commandCtx.Done():
					return
				}
			}
		}
	}()
	pollDone := make(chan struct{})
	go func(receiver Receiver) { defer close(pollDone); receiver.poll(ctx, offset, events) }(r)
	ticker := time.NewTicker(telegram.HealthInterval(r.Config.Limits))
	defer ticker.Stop()
	var activeCancel context.CancelFunc
	var done chan turnResult
	var history []conversation.Event
	var pendingControl *controlNotice
	tone := &chatstyle.Dialogue{}
	defer func() {
		cancel()
		cancelCommands()
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
		<-commandsDone
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
	reply := func(receipt *telegram.Receipt, text string) {
		if err := r.send(ctx, receipt, text); err == nil {
			health.LastReplyAt = time.Now().UTC()
			receipt.State = "completed"
			if err = telegram.Record(r.Root, *receipt, true); err != nil {
				r.record(ctx, "receipt_failed", receipt.UpdateID, receipt.SessionID)
			}
		}
	}
	finishTurn := func(result turnResult) {
		activeUpdate := health.ActiveUpdate
		activeCancel()
		activeCancel = nil
		done = nil
		history = result.history
		health.ActiveUpdate = 0

		var recoveryErr error
		recoveryID := ""
		if result.uncertain || pendingControl != nil {
			recoveryErr = Reconcile(ctx, r.Root, r.Config.Limits)
			if recoveryErr != nil {
				health.AIBlocked = true
				recoveryID = r.record(ctx, "recovery_blocked", activeUpdate, health.SessionID)
			}
		}
		if result.uncertain {
			history = append(history, conversation.Event{
				Role:    "host",
				Content: "The previous request could not be confirmed. Do not automatically rerun existing work; wait for the user's next request.",
			})
		}
		if pendingControl != nil {
			control := pendingControl
			pendingControl = nil
			switch {
			case recoveryErr != nil:
				text := "The previous execution cleanup could not be confirmed, so a new AI request cannot start. /status and /errors remain available."
				if recoveryID != "" {
					text += "\nError ID: " + recoveryID
				}
				reply(&control.receipt, text)
			case result.uncertain:
				text := "The previous request or action result could not be confirmed. It was not retried automatically. Check /jobs and /errors."
				if control.reset {
					history = nil
					health.SessionID = files.ID()
					text += "\nA new chat has started."
				}
				reply(&control.receipt, text)
			case result.cancelled:
				if control.reset {
					history = nil
					health.SessionID = files.ID()
					reply(&control.receipt, "The previous execution cleanup finished and a new chat has started.")
				} else {
					reply(&control.receipt, "The current reply was stopped. Check /jobs for work that may already have started.")
				}
			case result.err == nil:
				if control.reset {
					history = nil
					health.SessionID = files.ID()
					reply(&control.receipt, "The earlier reply had already finished, and a new chat has started.")
				} else {
					reply(&control.receipt, "The earlier reply had already finished, so it was not stopped.")
				}
			default:
				text := "The earlier reply had already ended. Check /errors for the cause and recovery guidance."
				if control.reset {
					history = nil
					health.SessionID = files.ID()
					text += "\nA new chat has started."
				}
				reply(&control.receipt, text)
			}
		}
		writeHealth()
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = r.Errors.Flush(ctx)
			writeHealth()
		case result := <-done:
			finishTurn(result)
		case result := <-commandResults:
			if ctx.Err() != nil {
				continue
			}
			if result.err != nil {
				result.receipt.ErrorID = r.record(ctx, "host_failed", result.receipt.UpdateID, result.receipt.SessionID)
				if strings.TrimSpace(result.response) == "" {
					result.response = "An internal action could not be completed. Check /errors."
				}
				result.response += "\nError ID: " + result.receipt.ErrorID
			}
			reply(&result.receipt, result.response)
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
			current, configErr := config.Load(r.Root)
			if configErr == nil {
				r.Config = current
			}
			fields := strings.Fields(text)
			if active, guardErr := updateguard.Active(r.Root, r.Config.Limits.MaxArtifactBytes); guardErr != nil {
				reply(&receipt, "Self-update state could not be read. Inspect the local update state before another apply.")
				continue
			} else if active && !(len(fields) > 0 && (fields[0] == "/status" || fields[0] == "/errors" || (fields[0] == "/update" && len(fields) == 2 && fields[1] == "status"))) {
				reply(&receipt, "Self-update activation is in progress. /status, /errors, and /update status remain available.")
				continue
			}
			if response, handled, languageErr := chatlanguage.Handle(r.Root, r.Config.Limits, text); handled {
				tone = &chatstyle.Dialogue{}
				if languageErr != nil {
					receipt.ErrorID = r.record(ctx, reception.ErrorCode(languageErr, r.Config), u.ID, health.SessionID)
					response = reception.FailureMessage(languageErr) + "\nError ID: " + receipt.ErrorID
				}
				reply(&receipt, response)
				continue
			}
			if response, handled, toneErr := tone.Handle(r.Root, r.Config.Limits, text); handled {
				if toneErr != nil {
					receipt.ErrorID = r.record(ctx, reception.ErrorCode(toneErr, r.Config), u.ID, health.SessionID)
					response = reception.FailureMessage(toneErr) + "\nError ID: " + receipt.ErrorID
				}
				reply(&receipt, response)
				continue
			}
			if fields := strings.Fields(text); len(fields) > 0 && fields[0] == "/update" {
				parts := strings.Fields(text)
				if len(parts) > 2 || (len(parts) == 2 && parts[1] != "check" && parts[1] != "status") {
					reply(&receipt, "Usage: /update check, /update status, or /update")
					continue
				}
				if len(parts) == 2 && parts[1] == "check" {
					result, updateErr := selfupdate.Check(ctx, r.Root, r.Config.Limits.MaxArtifactBytes)
					if updateErr != nil {
						reply(&receipt, "Update check could not be completed. Use /update status for the retained safe outcome.")
						continue
					}
					if result.Available {
						reply(&receipt, "An owner-configured update is available: "+result.Revision[:12]+".")
					} else {
						reply(&receipt, "The installed revision is current.")
					}
					continue
				}
				if len(parts) == 2 && parts[1] == "status" {
					result, updateErr := selfupdate.Inspect(r.Root, r.Config.Limits.MaxArtifactBytes)
					if updateErr != nil {
						reply(&receipt, "Update status could not be read. Inspect the local update state before another apply.")
						continue
					}
					if !result.Configured {
						reply(&receipt, "Self-update is not configured on this host.")
					} else if result.State != nil {
						reply(&receipt, updateStatusMessage(result.State.Status, result.State.Message))
					} else {
						reply(&receipt, "Self-update is configured; no update has run yet.")
					}
					continue
				}
				// The completion of this first reply is the durable receipt handshake.
				// Only then can the detached unit read it and begin an activation.
				if err := r.send(ctx, &receipt, "Update request accepted. The chat will stay available while the candidate is fetched and built."); err != nil {
					continue
				}
				receipt.State = "completed"
				if err := telegram.Record(r.Root, receipt, true); err != nil {
					r.record(ctx, "receipt_failed", u.ID, health.SessionID)
					continue
				}
				if _, err := selfupdate.Launch(ctx, r.Root, u.ID, r.Binding.ChatID); err != nil {
					reply(&receipt, "The independent update service could not be started. Use /update status before trying again.")
				}
				continue
			}
			if text == "/cancel" || text == "취소" || text == "/reset" || text == "/새대화" {
				reset := text == "/reset" || text == "/새대화"
				if activeCancel != nil {
					select {
					case result := <-done:
						finishTurn(result)
					default:
					}
				}
				if activeCancel != nil {
					if pendingControl == nil {
						pendingControl = &controlNotice{receipt: receipt, reset: reset}
						activeCancel()
					} else {
						pendingControl.reset = pendingControl.reset || reset
						reply(&receipt, "Stopping the current reply has already been requested.")
					}
				} else if reset {
					if e := Reconcile(ctx, r.Root, r.Config.Limits); e != nil {
						health.AIBlocked = true
						receipt.ErrorID = r.record(ctx, "recovery_blocked", u.ID, health.SessionID)
						reply(&receipt, "The previous execution cleanup could not be confirmed, so a new chat did not start. /status and /errors remain available.\nError ID: "+receipt.ErrorID)
					} else {
						health.AIBlocked = false
						history = nil
						health.SessionID = files.ID()
						reply(&receipt, "A new chat has started.")
					}
				} else {
					reply(&receipt, "There is no reply in progress.")
				}
				writeHealth()
				continue
			}
			if reception.RequiresRuntimeWait(text) {
				select {
				case commands <- commandRequest{receipt: receipt, text: text, config: r.Config}:
				case <-ctx.Done():
					return nil
				default:
					reply(&receipt, "Management commands are temporarily at capacity. Check /status and resend the command after the current command completes.")
				}
				continue
			}
			host := reception.Host{Root: r.Root, Config: r.Config}
			response, handled, commandErr := host.Command(ctx, reception.Telegram, text)
			if handled {
				if commandErr != nil {
					receipt.ErrorID = r.record(ctx, "host_failed", u.ID, health.SessionID)
					if strings.TrimSpace(response) == "" {
						response = "An internal action could not be completed. Check /errors."
					}
					response += "\nError ID: " + receipt.ErrorID
				}
				reply(&receipt, response)
				continue
			}
			if int64(len(text)) > r.Config.Limits.MaxSourceBytes {
				reply(&receipt, "The input is too long. Send it in smaller parts.")
				continue
			}
			if activeCancel != nil {
				reply(&receipt, "An earlier request is still being processed. Wait for its reply or use /cancel before sending a new request. Fixed management commands remain available.")
				continue
			}
			if health.AIBlocked || configErr != nil {
				reply(&receipt, "AI execution recovery is required. Check /status and /errors. Fixed management commands remain available.")
				continue
			}
			if e = r.acknowledge(ctx, &receipt, u.Message.ID); e != nil {
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

func updateStatusMessage(status, reason string) string {
	if status == "completed" {
		return "Update status: completed."
	}
	if status == "recovery_required" {
		return "Update status: recovery is required locally before another update."
	}
	switch reason {
	case "schema_rejected":
		return "Update status: refused because the candidate changes the database schema or migrations."
	case "candidate_build_failed":
		return "Update status: the candidate could not be built; installed services were not changed."
	case "busy":
		return "Update status: deferred because work or chat activity was still in progress."
	case "rollback_failed":
		return "Update status: rollback could not be confirmed; local recovery is required."
	case "interrupted":
		return "Update status: interrupted; inspect local recovery before another update."
	case "rolled_back":
		return "Update status: the candidate failed and the prior binaries were restored."
	default:
		return "Update status: failed before completion."
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
