package reception

import (
	"context"
	"encoding/json"
	"strings"

	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
)

const Help = "할 일을 자연스럽게 말씀해 주세요. AI가 응답하지 않아도 아래 명령은 동작합니다.\n/status 상태 · /errors 오류 보고 · /ack 오류ID 확인 · /jobs 작업 목록\n/pause 업무 큐 중지 · /resume 큐 재개 · /cancel 작업ID 취소 · /retry 작업ID 재시도\n/worker start 업무 실행기 시작 · /worker stop 실행기 중지\n/features 설치 가능 기능 · /install 기능ID 설치 · /guide 서비스ID 안내\n/cancel 현재 답변 중단 · /reset 새 대화 · /help 사용법\n계정 인증과 비밀 입력은 호스트의 로컬/SSH 설정에서 진행합니다."

// Command returns handled=false only for ordinary conversation. Unknown slash
// commands are answered mechanically so they cannot accidentally become actions.
func (h Host) Command(ctx context.Context, channel, request string) (string, bool, error) {
	parts := strings.Fields(request)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return "", false, nil
	}
	name := strings.ToLower(parts[0])
	if name == "/worker" {
		if len(parts) != 2 || (parts[1] != "start" && parts[1] != "stop") {
			return "사용법: /worker start 또는 /worker stop. 실제 상태는 /status로 확인하세요.", true, nil
		}
		action := conversation.StartWorker
		if parts[1] == "stop" {
			action = conversation.StopWorker
		}
		result, err := h.Dispatch(ctx, channel, conversation.Action{Name: action}, request, map[string]bool{})
		if err != nil {
			return "", true, err
		}
		raw, err := json.MarshalIndent(result, "", "  ")
		return string(raw), true, err
	}
	if name == "/help" || name == "/start" {
		return Help, true, nil
	}
	action := conversation.Action{}
	argc := 1
	switch name {
	case "/status":
		action.Name = conversation.RuntimeStatus
	case "/errors":
		action.Name = conversation.ListErrors
	case "/ack":
		action.Name = conversation.AcknowledgeError
		argc = 2
	case "/jobs":
		action.Name = conversation.ListJobs
	case "/pause":
		action.Name = conversation.PauseQueue
	case "/resume":
		action.Name = conversation.ResumeQueue
	case "/cancel":
		action.Name = conversation.CancelJob
		argc = 2
	case "/retry":
		action.Name = conversation.RetryJob
		argc = 2
	case "/features":
		action.Name = conversation.ListFeatures
	case "/install":
		action.Name = conversation.InstallFeature
		argc = 2
	case "/guide":
		action.Name = conversation.ReadGuide
		argc = 2
	default:
		return "알 수 없는 명령입니다. /help로 사용법을 확인하세요.", true, nil
	}
	if len(parts) != argc {
		return "명령 인자를 확인해 주세요. /help로 사용법을 확인할 수 있습니다.", true, nil
	}
	if argc == 2 {
		if name == "/install" || name == "/guide" {
			action.Service = parts[1]
		} else {
			action.Reference = parts[1]
		}
	}
	result, err := h.Dispatch(ctx, channel, action, request, map[string]bool{})
	if err != nil {
		return "", true, err
	}
	if action.Name == conversation.ListErrors {
		reports := result.Detail.([]errorreport.Report)
		if len(reports) == 0 {
			return "누적된 운영 오류가 없습니다.", true, nil
		}
		var text strings.Builder
		for _, report := range reports {
			// Compact summaries fit the channel budget; CLI show retains details.
			text.WriteString(report.Summary + "\nID: " + report.ID + "\n")
			counts, _ := json.Marshal(map[string]uint64{"전체": report.Count, "미확인": report.Count - report.Acknowledged})
			text.Write(counts)
			text.WriteString("\n" + report.Recovery + "\n\n")
		}
		return text.String(), true, nil
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	return string(raw), true, err
}
