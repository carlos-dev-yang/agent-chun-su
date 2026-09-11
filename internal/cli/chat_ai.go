package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"chunsu/internal/conversation"
	"chunsu/internal/onboarding"
	"chunsu/internal/reception"
)

var errReceptionSteered = errors.New("reception input changed")

func (d *setupDialogue) runAI(initial string) error {
	if d.config.Executor.Kind == "" || d.config.Executor.Path == "" || d.config.Executor.Model == "" {
		return errors.New("대화 실행기를 먼저 설정해 주세요. AI 없이 설정하려면 chat --guided를 사용할 수 있습니다.")
	}
	session := reception.New(d.root, d.config, reception.Local)
	fmt.Fprintln(d.out, "춘수와 대화합니다. 할 일을 편하게 말씀해 주세요. 실행은 지원되는 작업으로 제한됩니다.")
	fmt.Fprintln(d.out, "대화는 설정된 Codex로 전달됩니다. 비밀값은 입력하지 마세요. 취소: 현재 답변 중단, /새대화: 맥락 초기화, 종료: 끝내기.")
	for {
		request := initial
		initial = ""
		if request == "" {
			var err error
			request, err = d.ask("")
			if errors.Is(err, errSetupBack) {
				continue
			}
			if err != nil {
				return err
			}
		}
		if request == "/새대화" || request == "/reset" {
			session = reception.New(d.root, d.config, reception.Local)
			fmt.Fprintln(d.out, "새 대화를 시작했습니다.")
			continue
		}
		if strings.TrimSpace(request) == "" {
			continue
		}
		generate := func(ctx context.Context, directory string, prompt, schema, skill []byte) (reception.Generation, error) {
			result, steering, err := d.generate(directory, prompt, schema, skill)
			if steering != "" {
				initial = steering
				return result, errReceptionSteered
			}
			return result, err
		}
		emit := func(event reception.Event) error {
			switch event.Kind {
			case "thinking":
				fmt.Fprintln(d.out, "춘수가 생각하고 있습니다…")
			case "reply":
				fmt.Fprintln(d.out, "춘수:", event.Text)
			case "action":
				fmt.Fprintln(d.out, "[호스트 처리 결과]", event.Action, ":", event.Text)
			}
			return nil
		}
		err := session.Turn(d.ctx, request, d.receptionHost(), generate, emit)
		if errors.Is(err, reception.ErrUncertain) || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, errReceptionSteered) {
			continue
		}
		if errors.Is(err, errSetupBack) {
			session.History = append(session.History, conversation.Event{Role: "host", Content: "사용자가 답변을 취소했습니다. 미완료 작업을 자동 재실행하지 마세요."})
			fmt.Fprintln(d.out, "답변을 중단했습니다.")
		} else if err != nil {
			fmt.Fprintln(d.out, err)
		}
	}
}

func (d *setupDialogue) receptionHost() reception.Host {
	return reception.Host{
		Root: d.root, Config: d.config,
		GuideInstalled: func(result onboarding.Result) {
			fmt.Fprintln(d.out, "[로컬 설치]", result.Message, "\n", result.ManualPath)
		},
		DisplayReport: func(ctx context.Context, b []byte) error {
			fmt.Fprintln(d.out, "[사용자에게만 표시하는 보존 보고서]")
			_, err := d.out.Write(b)
			return err
		},
		GmailSetup: func(ctx context.Context) (reception.HostResult, error) {
			d.lastSetup = nil
			prepared, err := d.call("prepare", onboarding.Request{Service: "gmail"})
			if err != nil {
				return reception.HostResult{}, err
			}
			fmt.Fprintln(d.out, "[로컬 Gmail 매뉴얼]", prepared.ManualPath)
			fmt.Fprintln(d.out, "[로컬 인증 단계] 여기서 입력한 계정·파일 경로와 인증 결과의 비밀값은 AI에게 보내지 않습니다.")
			err = d.gmail()
			if errors.Is(err, errSetupBack) {
				return reception.HostResult{Status: "cancelled", Detail: "사용자가 로컬 설정을 취소했습니다."}, nil
			}
			if err != nil {
				return reception.HostResult{}, err
			}
			if d.lastSetup == nil {
				return reception.HostResult{Status: "not_started", Detail: "로컬 설정 단계에서 인증 실행을 선택하지 않았습니다."}, nil
			}
			return reception.HostResult{Status: d.lastSetup.Status, Detail: "호스트의 Gmail 설정 결과입니다. 계정·비밀·인증 URL은 전달하지 않았습니다."}, nil
		},
	}
}

// New input cancels the old generation before any proposal from it is dispatched.
func (d *setupDialogue) generate(directory string, prompt, schema, skill []byte) (reception.Generation, string, error) {
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
	select {
	case result := <-done:
		return result.result, "", result.err
	case <-d.ctx.Done():
		cancel()
		result := <-done
		return result.result, "", d.ctx.Err()
	case line, ok := <-d.lines:
		cancel()
		result := <-done
		if !ok {
			return result.result, "", io.EOF
		}
		if line.err != nil {
			return result.result, "", line.err
		}
		switch strings.ToLower(line.text) {
		case "종료", "그만", "quit", "exit":
			return result.result, "", io.EOF
		case "취소", "뒤로", "cancel", "back":
			return result.result, "", errSetupBack
		}
		if line.text == "" {
			return result.result, "", errSetupBack
		}
		return result.result, line.text, nil
	}
}
