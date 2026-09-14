package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"chunsu/internal/chatlanguage"
	"chunsu/internal/chatstyle"
	"chunsu/internal/config"
	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/onboarding"
	"chunsu/internal/reception"
)

var errReceptionSteered = errors.New("reception input changed")

func (d *setupDialogue) runAI(initial string) error {
	session := reception.New(d.root, d.config, reception.Local)
	tone := &chatstyle.Dialogue{}
	reports := errorreport.New(d.root, d.config.Limits)
	aiBlocked := false
	fmt.Fprintln(d.out, "Chat with Chun-su naturally. Execution remains limited to supported work.")
	fmt.Fprintln(d.out, "Conversation is sent to the configured Codex. Do not enter secrets. cancel stops the current reply, /reset clears context, and exit leaves chat.")
	for {
		request := initial
		initial = ""
		if request == "" {
			var err error
			request, err = d.askInput("", tone.Pending())
			if errors.Is(err, errSetupBack) {
				continue
			}
			if err != nil {
				return err
			}
		}
		if current, err := config.Load(d.root); err == nil {
			d.config = current
			session.Config = current
		}
		if response, handled, err := chatlanguage.Handle(d.root, d.config.Limits, request); handled {
			tone = &chatstyle.Dialogue{}
			if err != nil {
				id, _ := reports.Record(d.ctx, reception.ErrorCode(err, d.config), errorreport.Correlation{SessionID: session.ID})
				fmt.Fprintln(d.out, reception.FailureMessage(err), "Error ID:", id)
			} else {
				fmt.Fprintln(d.out, response)
			}
			continue
		}
		if response, handled, err := tone.Handle(d.root, d.config.Limits, request); handled {
			if err != nil {
				id, _ := reports.Record(d.ctx, reception.ErrorCode(err, d.config), errorreport.Correlation{SessionID: session.ID})
				fmt.Fprintln(d.out, reception.FailureMessage(err), "Error ID:", id)
			} else {
				fmt.Fprintln(d.out, response)
			}
			continue
		}
		if request == "/새대화" || request == "/reset" {
			if err := session.Recover(d.ctx); err != nil {
				fmt.Fprintln(d.out, "The previous execution cleanup could not be confirmed. Check /errors and host recovery state.")
				continue
			}
			aiBlocked = false
			session = reception.New(d.root, d.config, reception.Local)
			fmt.Fprintln(d.out, "A new chat has started.")
			continue
		}
		if strings.TrimSpace(request) == "/cancel" {
			fmt.Fprintln(d.out, "There is no reply in progress.")
			continue
		}
		if strings.TrimSpace(request) == "" {
			continue
		}
		if response, handled, err := d.receptionHost().Command(d.ctx, reception.Local, request); handled {
			if err != nil {
				id, _ := reports.Record(d.ctx, "host_failed", errorreport.Correlation{SessionID: session.ID})
				if strings.TrimSpace(response) == "" {
					response = "An internal action could not be completed. Check /errors."
				}
				fmt.Fprintln(d.out, response, "Error ID:", id)
			} else {
				fmt.Fprintln(d.out, response)
			}
			continue
		}
		if aiBlocked {
			fmt.Fprintln(d.out, "AI execution cleanup is required. Check /reset and /errors for recovery state. Fixed management commands remain available.")
			continue
		}
		fmt.Fprintln(d.out, reception.Acknowledgment)
		generate := func(ctx context.Context, directory string, prompt, schema, skill []byte) (reception.Generation, error) {
			result, steering, err := d.generate(directory, prompt, schema, skill, func(text string) bool {
				response, handled, languageErr := chatlanguage.Handle(d.root, d.config.Limits, text)
				if handled {
					tone = &chatstyle.Dialogue{}
					if languageErr != nil {
						id, _ := reports.Record(d.ctx, reception.ErrorCode(languageErr, d.config), errorreport.Correlation{SessionID: session.ID})
						fmt.Fprintln(d.out, reception.FailureMessage(languageErr), "Error ID:", id)
					} else {
						fmt.Fprintln(d.out, response)
					}
					return true
				}
				reserved := strings.TrimSpace(text)
				if reserved == "/cancel" || reserved == "/reset" || reserved == "/새대화" {
					return false
				}
				if !strings.HasPrefix(reserved, "/") {
					return false
				}
				response, handled, commandErr := d.receptionHost().Command(d.ctx, reception.Local, text)
				if !handled {
					return false
				}
				if commandErr != nil {
					id, _ := reports.Record(d.ctx, "host_failed", errorreport.Correlation{SessionID: session.ID})
					if strings.TrimSpace(response) == "" {
						response = "An internal action could not be completed. Check /errors."
					}
					fmt.Fprintln(d.out, response, "Error ID:", id)
				} else {
					fmt.Fprintln(d.out, response)
				}
				return true
			})
			if steering != "" {
				initial = steering
				return result, errReceptionSteered
			}
			return result, err
		}
		emit := func(event reception.Event) error {
			if !event.UserVisible() {
				return nil
			}
			_, err := fmt.Fprintln(d.out, "Chun-su:", event.Text)
			return err
		}
		err := session.Turn(d.ctx, request, d.receptionHost(), generate, emit)
		if errors.Is(err, reception.ErrUncertain) {
			aiBlocked = session.Recover(d.ctx) != nil
			session.History = append(session.History, conversation.Event{
				Role:    "host",
				Content: "The previous request could not be confirmed. Do not automatically rerun existing work; wait for the user's next request.",
			})
			id, _ := reports.Record(d.ctx, "execution_uncertain", errorreport.Correlation{SessionID: session.ID})
			fmt.Fprintln(d.out, reception.FailureMessage(err), "Error ID:", id)
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return err
			}
		} else if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return err
		} else if errors.Is(err, errReceptionSteered) {
			continue
		} else if errors.Is(err, errSetupBack) {
			session.History = append(session.History, conversation.Event{Role: "host", Content: "The user stopped the reply. Do not automatically rerun incomplete work."})
			fmt.Fprintln(d.out, "The reply was stopped.")
		} else if err != nil {
			code := reception.ErrorCode(err, d.config)
			if errors.Is(err, reception.ErrUncertain) {
				code = "execution_uncertain"
				aiBlocked = session.Recover(d.ctx) != nil
			}
			id, _ := reports.Record(d.ctx, code, errorreport.Correlation{SessionID: session.ID})
			fmt.Fprintln(d.out, reception.FailureMessage(err), "Error ID:", id)
		}
	}
}

func (d *setupDialogue) receptionHost() reception.Host {
	return reception.Host{
		Root: d.root, Config: d.config,
		GuideInstalled: func(result onboarding.Result) {
			fmt.Fprintln(d.out, "[Local installation]", result.Message, "\n", result.ManualPath)
		},
		DisplayReport: func(ctx context.Context, b []byte) error {
			fmt.Fprintln(d.out, "[Preserved report shown only to the user]")
			_, err := d.out.Write(b)
			return err
		},
		GmailSetup: func(ctx context.Context) (reception.HostResult, error) {
			d.lastSetup = nil
			prepared, err := d.call("prepare", onboarding.Request{Service: "gmail"})
			if err != nil {
				return reception.HostResult{}, err
			}
			fmt.Fprintln(d.out, "[Local Gmail manual]", prepared.ManualPath)
			fmt.Fprintln(d.out, "[Local authorization step] Account details, file paths, and secrets from authorization results entered here are not sent to the AI.")
			err = d.gmail()
			if errors.Is(err, errSetupBack) {
				return reception.HostResult{Status: "cancelled", Detail: "The user cancelled local setup."}, nil
			}
			if err != nil {
				return reception.HostResult{}, err
			}
			if d.lastSetup == nil {
				return reception.HostResult{Status: "not_started", Detail: "Authorization was not started in local setup."}, nil
			}
			return reception.HostResult{Status: d.lastSetup.Status, Detail: "This is the host's Gmail setup result. Account data, secrets, and authorization URLs were not shared."}, nil
		},
	}
}

// New input cancels the old generation before any proposal from it is dispatched.
// A recognized control may instead be handled while the generation retains its snapshot.
func (d *setupDialogue) generate(directory string, prompt, schema, skill []byte, control func(string) bool) (reception.Generation, string, error) {
	ctx, cancel := context.WithCancel(d.ctx)
	defer cancel()
	type completed struct {
		result reception.Generation
		err    error
	}
	done := make(chan completed, 1)
	go func() {
		r, e := reception.Generate(ctx, d.root, directory, d.config, prompt, schema, skill)
		done <- completed{r, e}
	}()
	for {
		select {
		case result := <-done:
			return result.result, "", result.err
		case <-d.ctx.Done():
			cancel()
			result := <-done
			return result.result, "", d.ctx.Err()
		case line, ok := <-d.lines:
			if !ok {
				cancel()
				result := <-done
				return result.result, "", io.EOF
			}
			if line.err != nil {
				cancel()
				result := <-done
				return result.result, "", line.err
			}
			if control != nil && control(line.text) {
				continue
			}
			cancel()
			result := <-done
			switch strings.ToLower(line.text) {
			case "종료", "그만", "quit", "exit":
				return result.result, "", io.EOF
			case "취소", "뒤로", "cancel", "/cancel", "back":
				return result.result, "", errSetupBack
			}
			if line.text == "" {
				return result.result, "", errSetupBack
			}
			return result.result, line.text, nil
		}
	}
}
