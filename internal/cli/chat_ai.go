package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/control"
	"chunsu/internal/conversation"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/onboarding"
	"chunsu/internal/store"
)

type chatHostResult struct {
	Status string `json:"status"`
	Detail any    `json:"detail,omitempty"`
}

func (d *setupDialogue) runAI(initial string) error {
	if d.config.Executor.Kind == "" || d.config.Executor.Path == "" || d.config.Executor.Model == "" {
		return errors.New("대화 실행기를 먼저 설정해 주세요. 기존 Codex의 executor.kind/path/model을 사용하며, AI 없이 설정하려면 chat --guided를 사용할 수 있습니다.")
	}
	schema, err := conversation.Schema()
	if err != nil {
		return err
	}
	skill, err := conversation.Skill()
	if err != nil {
		return err
	}
	session := files.ID()
	knownReports := map[string]bool{}
	history := []conversation.Event{}
	fmt.Fprintln(d.out, "춘수와 대화합니다. 할 일을 편하게 말씀해 주세요. 실행은 지원되는 작업으로 제한됩니다.")
	fmt.Fprintln(d.out, "대화는 설정된 Codex로 전달됩니다. 비밀값은 입력하지 마세요. 취소: 현재 답변 중단, /새대화: 맥락 초기화, 종료: 끝내기.")
	for {
		request := initial
		initial = ""
		if request == "" {
			request, err = d.ask("")
			if errors.Is(err, errSetupBack) {
				continue
			}
			if err != nil {
				return err
			}
		}
		if request == "/새대화" || request == "/reset" {
			history = nil
			knownReports = map[string]bool{}
			session = files.ID()
			fmt.Fprintln(d.out, "새 대화를 시작했습니다.")
			continue
		}
		if strings.TrimSpace(request) == "" {
			continue
		}
		history = append(history, conversation.Event{Role: "user", Content: request})
		steps := 0
		for steps < min(conversation.MaxActionsPerTurn, d.config.Limits.MaxToolCalls) {
			prompt, e := conversation.Prompt(history, d.config.Limits.MaxSourceBytes)
			if e != nil {
				fmt.Fprintln(d.out, e)
				break
			}
			directory := filepath.Join(d.root, "chat", session, files.ID())
			fmt.Fprintln(d.out, "춘수가 생각하고 있습니다…")
			result, steering, e := d.generate(directory, prompt, schema, skill)
			if result.Outcome == "orphaned" {
				return errors.New("대화 실행기 프로세스 정리를 확인해야 합니다. 채팅을 종료합니다.")
			}
			if steering != "" {
				if steering == "/새대화" || steering == "/reset" {
					history = nil
					knownReports = map[string]bool{}
					session = files.ID()
					fmt.Fprintln(d.out, "새 대화를 시작했습니다.")
					break
				}
				request = steering
				history = append(history, conversation.Event{Role: "user", Content: steering})
				continue
			}
			if errors.Is(e, io.EOF) || errors.Is(e, context.Canceled) {
				return e
			}
			if errors.Is(e, errSetupBack) {
				fmt.Fprintln(d.out, "답변을 중단했습니다. 제안된 작업은 실행하지 않았습니다.")
				break
			}
			if e != nil {
				fmt.Fprintln(d.out, e)
				break
			}
			reply, e := conversation.Decode(result.Final)
			if e != nil {
				fmt.Fprintln(d.out, "실행기의 답변이 허용된 형식이나 작업 목록과 맞지 않아 처리하지 않았습니다.")
				break
			}
			fmt.Fprintln(d.out, "춘수:", reply.Message)
			encoded, _ := json.Marshal(reply)
			history = append(history, conversation.Event{Role: "assistant", Content: string(encoded)})
			if reply.Action.Name == conversation.None {
				break
			}
			intent, _ := json.Marshal(map[string]any{"action": reply.Action, "state": "requested"})
			if e = files.Write(directory, "action.json", intent, false); e != nil {
				return e
			}
			outcome, actionErr := d.chatAction(reply.Action, request, knownReports)
			if actionErr != nil {
				// Local diagnostics may contain paths or account information.
				// Display them locally; return only a bounded summary to the model.
				fmt.Fprintln(d.out, "[로컬 오류]", actionErr)
				outcome = chatHostResult{Status: "failed", Detail: "호스트 작업을 완료하지 못했습니다. 오류는 사용자 터미널에 표시했습니다. 자동 재시도하지 마세요."}
			}
			outcomeBytes, _ := json.Marshal(outcome)
			audit, _ := json.Marshal(map[string]any{"action": reply.Action, "state": outcome.Status, "result_digest": files.Digest(outcomeBytes)})
			if e = files.Write(directory, "action.json", audit, true); e != nil {
				return errors.New("작업 처리 후 기록 저장에 실패했습니다. 자동 재시도하지 말고 현재 상태를 확인하세요.")
			}
			if errors.Is(actionErr, io.EOF) || errors.Is(actionErr, context.Canceled) {
				return actionErr
			}
			history = append(history, conversation.Event{Role: "host", Content: string(outcomeBytes)})
			fmt.Fprintln(d.out, "[호스트 처리 결과]", reply.Action.Name, ":", outcome.Status)
			steps++
			if outcome.Status == "failed" {
				fmt.Fprintln(d.out, outcome.Detail)
				break
			}
		}
		if steps >= min(conversation.MaxActionsPerTurn, d.config.Limits.MaxToolCalls) {
			fmt.Fprintln(d.out, "이번 요청의 작업 처리 한도에 도달했습니다. 현재 결과를 바탕으로 이어서 요청할 수 있습니다.")
		}
	}
}

// New input cancels the old generation before any proposal from it is dispatched.
func (d *setupDialogue) generate(directory string, prompt, schema, skill []byte) (executor.Result, string, error) {
	ctx, cancel := context.WithCancel(d.ctx)
	defer cancel()
	type completed struct {
		result executor.Result
		err    error
	}
	done := make(chan completed, 1)
	go func() {
		r, e := executor.Converse(ctx, d.root, directory, d.config.Executor, d.config.Limits, prompt, schema, skill)
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

func (d *setupDialogue) chatAction(action conversation.Action, userRequest string, knownReports map[string]bool) (chatHostResult, error) {
	// Validate again at dispatch. The schema and model text never grant permissions.
	if err := conversation.Validate(conversation.Reply{Message: "dispatch", Action: action}); err != nil {
		return chatHostResult{}, err
	}
	switch action.Name {
	case conversation.RuntimeStatus:
		b, handled, err := control.Call(d.ctx, d.root, time.Duration(d.config.Limits.LockWaitSeconds)*time.Second, d.config.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
		if err != nil {
			return chatHostResult{}, err
		}
		if !handled {
			return chatHostResult{}, errors.New("호스트가 실행 중이지 않습니다")
		}
		var status struct {
			ActiveJob   string `json:"active_job"`
			Owner       string `json:"owner"`
			QueuePaused bool   `json:"queue_paused"`
		}
		if err = json.Unmarshal(b, &status); err != nil {
			return chatHostResult{}, err
		}
		return chatHostResult{Status: "observed", Detail: status}, nil
	case conversation.ReadGuide:
		b, err := conversation.Guide(action.Service)
		if err != nil {
			return chatHostResult{}, err
		}
		if int64(len(b)) > d.config.Limits.MaxSourceBytes {
			return chatHostResult{}, errors.New("매뉴얼이 대화 입력 한도를 초과합니다")
		}
		return chatHostResult{Status: "guide_read", Detail: string(b)}, nil
	case conversation.InstallGuide:
		result, err := d.call("prepare", onboarding.Request{Service: action.Service})
		if err != nil {
			return chatHostResult{}, err
		}
		fmt.Fprintln(d.out, "[로컬 설치]", result.Message, "\n", result.ManualPath)
		return chatHostResult{Status: result.Status, Detail: map[string]string{"service": action.Service, "pack_digest": result.PackDigest, "message": result.Message}}, nil
	case conversation.GmailSetup:
		d.lastSetup = nil
		prepared, err := d.call("prepare", onboarding.Request{Service: "gmail"})
		if err != nil {
			return chatHostResult{}, err
		}
		fmt.Fprintln(d.out, "[로컬 Gmail 매뉴얼]", prepared.ManualPath)
		fmt.Fprintln(d.out, "[로컬 인증 단계] 여기서 입력한 계정·파일 경로와 인증 결과의 비밀값은 AI에게 보내지 않습니다.")
		err = d.gmail()
		if errors.Is(err, errSetupBack) {
			return chatHostResult{Status: "cancelled", Detail: "사용자가 로컬 설정을 취소했습니다."}, nil
		}
		if err != nil {
			return chatHostResult{}, err
		}
		if d.lastSetup == nil {
			return chatHostResult{Status: "not_started", Detail: "로컬 설정 단계에서 인증 실행을 선택하지 않았습니다."}, nil
		}
		return chatHostResult{Status: d.lastSetup.Status, Detail: "호스트의 Gmail 설정 결과입니다. 계정·비밀·인증 URL은 전달하지 않았습니다."}, nil
	case conversation.ListJobs:
		s, err := store.OpenReadOnly(d.ctx, d.root)
		if err != nil {
			return chatHostResult{}, err
		}
		defer s.Close()
		jobs, err := s.Jobs(d.ctx)
		if err != nil {
			return chatHostResult{}, err
		}
		type summary struct {
			ID        string `json:"id"`
			Workgroup string `json:"workgroup"`
			Status    string `json:"status"`
			CreatedAt int64  `json:"created_at"`
		}
		items := []summary{}
		for i, job := range jobs {
			if i >= d.config.Limits.MaxMessages {
				break
			}
			items = append(items, summary{job.ID, job.Workgroup, job.Status, job.CreatedAt})
			knownReports[job.ID] = true
		}
		return chatHostResult{Status: "observed", Detail: map[string]any{"jobs": items, "total": len(jobs)}}, nil
	case conversation.ShowReport:
		if !knownReports[action.Reference] && !strings.Contains(userRequest, action.Reference) {
			return chatHostResult{}, errors.New("먼저 작업 목록에서 확인한 ID나 사용자가 지정한 ID를 선택해야 합니다")
		}
		o := &options{root: d.root}
		cmd := o.report()
		cmd.SetContext(d.ctx)
		cmd.SetOut(d.out)
		fmt.Fprintln(d.out, "[사용자에게만 표시하는 보존 보고서]")
		if err := cmd.RunE(cmd, []string{action.Reference}); err != nil {
			return chatHostResult{}, err
		}
		return chatHostResult{Status: "displayed_locally", Detail: "검증된 보존 보고서를 사용자 터미널에 표시했습니다. 본문은 AI에게 전달하지 않았습니다. 내용을 읽거나 요약했다고 주장하지 마세요."}, nil
	default:
		return chatHostResult{}, errors.New("이 작업은 채팅 실행 범위에 없습니다. 추가 adapter 또는 권한 경로가 필요합니다")
	}
}
