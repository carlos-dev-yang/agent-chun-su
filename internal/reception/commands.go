package reception

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/telegram"
)

const Help = "Describe what you need in plain language. The commands below work even when AI replies are unavailable.\n/status current status · /errors error reports · /ack errorID acknowledge an error · /jobs job list\n/pause pause the queue · /resume resume the queue · /cancel jobID cancel a job · /retry jobID retry a job\n/worker start start the work runner · /worker stop stop the work runner\n/features available features · /install featureID install a feature · /guide serviceID setup guide\n/language auto · /language ko · /language en · /language ja · /language pt-BR · /언어 auto\n/tone options · /tone current · /tone reset · /말투 옵션 · /말투 현재 · /말투 초기화\n/cancel stop the current reply · /reset start a new conversation · /help show this help\nComplete account authentication and secret entry in the host's local or SSH configuration."

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
			return "Usage: /worker start or /worker stop. Check /status for the observed state.", true, nil
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
		return "Unknown command. Use /help to see available commands.", true, nil
	}
	if len(parts) != argc {
		return "Check the command arguments. Use /help for usage.", true, nil
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
			return "No retained operational errors.", true, nil
		}
		var text strings.Builder
		for _, report := range reports {
			// Compact summaries fit the channel budget; CLI show retains details.
			text.WriteString(report.Summary + "\nID: " + report.ID + "\n")
			fmt.Fprintf(&text, "Count: %d · Unacknowledged: %d", report.Count, report.Count-report.Acknowledged)
			text.WriteString("\n" + report.Recovery + "\n\n")
		}
		return text.String(), true, nil
	}
	if action.Name == conversation.RuntimeStatus {
		return readableStatus(result), true, nil
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	return string(raw), true, err
}

func readableStatus(result HostResult) string {
	detail, ok := result.Detail.(map[string]any)
	if !ok {
		return "Status could not be read. Check /errors."
	}
	controllerAvailable, _ := detail["controller_available"].(bool)
	chatAlive, _ := detail["chat_receiver_alive"].(bool)
	chatReadable, _ := detail["chat_health_readable"].(bool)
	status, statusKnown := detail["controller"].(controllerStatus)

	lines := []string{"Status"}
	if controllerAvailable && statusKnown {
		lines = append(lines, "Controller: available.")
		if status.QueuePaused {
			lines = append(lines, "Queue: paused.")
		} else {
			lines = append(lines, "Queue: accepting work.")
		}
		if status.ActiveJob == "" {
			lines = append(lines, "Active job: none.")
		} else {
			lines = append(lines, "Active job: "+status.ActiveJob+".")
		}
		if status.WorkerRunning {
			lines = append(lines, "Worker: running.")
		} else {
			lines = append(lines, "Worker: not running.")
		}
	} else {
		lines = append(lines, "Controller: unavailable or its status response could not be confirmed.", "Queue, active job, and worker state: unknown.")
	}
	if requested, known := detail["worker_requested"].(bool); known {
		if requested {
			lines = append(lines, "Worker supervision: requested.")
		} else {
			lines = append(lines, "Worker supervision: not requested.")
		}
	} else {
		lines = append(lines, "Worker supervision: unknown.")
	}

	health, healthKnown := detail["chat"].(telegram.Health)
	switch {
	case !chatReadable:
		lines = append(lines, "Chat receiver: health record could not be read.", "Polling and AI reply state: unknown.", "Recovery state: unknown.")
	case !healthKnown || health.Version == 0:
		lines = append(lines, "Chat receiver: no health record is present.", "Polling and AI reply state: unknown.", "Recovery state: unknown.")
	case !chatAlive:
		lines = append(lines, "Chat receiver: health is stale or the receiver has stopped.", "Polling and AI reply state: unknown.")
		if health.AIBlocked {
			lines = append(lines, "Recovery: blocked in the last recorded receiver state.")
		} else {
			lines = append(lines, "Recovery state: not current because receiver health is stale.")
		}
	default:
		lines = append(lines, "Chat receiver: alive.", "Polling: "+readablePoll(health.Poll)+".")
		if health.ActiveUpdate > 0 {
			lines = append(lines, "AI reply: active.")
		} else {
			lines = append(lines, "AI reply: no active request.")
		}
		if health.AIBlocked {
			lines = append(lines, "Recovery: blocked; fixed commands remain available.")
		} else {
			lines = append(lines, "Recovery: not blocked in the current receiver health record.")
		}
	}
	return strings.Join(lines, "\n")
}

func readablePoll(poll string) string {
	switch poll {
	case "connected":
		return "connected"
	case "connecting":
		return "connecting"
	case "poll_failed":
		return "failed"
	case "authentication_failed":
		return "authentication failed"
	case "poll_conflict":
		return "conflicted"
	default:
		return "unknown"
	}
}
