package reception

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"chunsu/internal/backend"
	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/files"
	"chunsu/internal/telegram"
	"chunsu/internal/updateguard"
	"chunsu/internal/workerconfig"
)

const Help = "Describe what you need in plain language. The commands below work even when AI replies are unavailable.\n/status current status · /errors error reports · /ack errorID acknowledge an error · /jobs job list\n/pause pause the queue · /resume resume the queue · /cancel jobID cancel a job · /retry jobID retry a job\n/controller start|stop|restart|status manage the controller · /worker start|stop|restart|status manage the work runner · /worker config [use reception|use review|model MODEL] inspect or save task AI settings\n/update check · /update status · /update fetch and apply the owner-configured update\n/features available features · /install featureID install a feature · /guide list manuals · /guide runtime process usage and recovery · /guide ID read a manual\n/language auto · /language ko · /language en · /language ja · /language pt-BR · /언어 auto\n/tone options · /tone current · /tone reset · /말투 옵션 · /말투 현재 · /말투 초기화\n/cancel stop the current reply · /reset start a new conversation · /help show this help\nComplete account authentication and secret entry in the host's local or SSH configuration."

// Command returns handled=false only for ordinary conversation. Unknown slash
// commands are answered mechanically so they cannot accidentally become actions.
func (h Host) Command(ctx context.Context, channel, request string) (string, bool, error) {
	parts := strings.Fields(request)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return "", false, nil
	}
	name := strings.ToLower(parts[0])
	if name != "/status" && name != "/errors" && name != "/help" && name != "/guide" && !(name == "/update" && len(parts) == 2 && parts[1] == "status") {
		if active, err := updateguard.Active(h.Root, h.Config.Limits.MaxArtifactBytes); err != nil {
			return "", true, err
		} else if active {
			return "Self-update activation is in progress. /status, /errors, and /update status remain available.", true, nil
		}
	}
	if name == "/worker" && len(parts) >= 2 && parts[1] == "config" {
		return h.workerConfigCommand(ctx, channel, request, parts)
	}
	if name == "/controller" || name == "/worker" {
		if channel != Local && channel != Telegram {
			return "This runtime command is unavailable in this reception channel.", true, nil
		}
		if len(parts) != 2 || !runtimeOperation(parts[1]) {
			return "Usage: " + name + " start, stop, restart, or status. Check /status for the observed state.", true, nil
		}
		component := strings.TrimPrefix(name, "/")
		result, err := h.runtime(ctx, component, parts[1])
		if err != nil {
			if strings.TrimSpace(result.Message) != "" {
				return result.Message, true, err
			}
			return runtimeFailure(component, parts[1]), true, err
		}
		if strings.TrimSpace(result.Message) == "" {
			return runtimeFailure(component, parts[1]), true, nil
		}
		if component == "worker" && parts[1] == "status" {
			return workerStatusMessage(result), true, nil
		}
		return result.Message, true, nil
	}
	if name == "/help" || name == "/start" {
		return Help, true, nil
	}
	if name == "/guide" && len(parts) == 1 {
		menu, err := conversation.ManualMenu()
		return menu, true, err
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
		if action.Name == conversation.RuntimeStatus {
			return readableStatus(result), true, err
		}
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
	if action.Name == conversation.ReadGuide {
		guide, ok := result.Detail.(string)
		if !ok {
			return "", true, errors.New("guide content is unavailable")
		}
		return guide, true, nil
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	return string(raw), true, err
}

func (h Host) workerConfigCommand(ctx context.Context, channel, request string, parts []string) (string, bool, error) {
	if channel != Local && channel != Telegram {
		return "This worker configuration command is unavailable in this reception channel.", true, nil
	}
	action := conversation.Action{Name: conversation.ReadWorkerConfig}
	switch len(parts) {
	case 2:
	case 4:
		switch parts[2] {
		case "use":
			action.Name, action.Service, action.Reference = conversation.ConfigureWorker, "source", parts[3]
		case "model":
			action.Name, action.Service, action.Reference = conversation.ConfigureWorker, "model", parts[3]
		default:
			return "Usage: /worker config [use reception|use review|model MODEL].", true, nil
		}
	default:
		return "Usage: /worker config [use reception|use review|model MODEL].", true, nil
	}
	result, err := h.Dispatch(ctx, channel, action, request, map[string]bool{})
	if err != nil {
		if configured, ok := result.Detail.(workerconfig.Result); ok && strings.TrimSpace(configured.Message) != "" {
			return configured.Message, true, err
		}
		if conversation.Validate(conversation.Reply{Message: "command", Action: action}) != nil {
			return "Usage: /worker config [use reception|use review|model MODEL].", true, nil
		}
		return "Worker configuration could not be completed. Check /errors.", true, err
	}
	configured, ok := result.Detail.(workerconfig.Result)
	if !ok {
		return "Worker configuration could not be read. Check /errors.", true, err
	}
	return workerConfigMessage(configured), true, err
}

func workerConfigMessage(result workerconfig.Result) string {
	settings := result.Settings
	lines := []string{"Worker configuration"}
	if settings.Configured {
		lines = append(lines, "Configured: yes.")
	} else {
		lines = append(lines, "Configured: no.")
	}
	if settings.Driver != "" {
		lines = append(lines, "Driver: "+settings.Driver+".")
	}
	if settings.Model != "" {
		lines = append(lines, "Model: "+settings.Model+".")
	}
	if settings.Environment != "" {
		lines = append(lines, "Environment: "+settings.Environment+".")
	}
	if len(settings.AvailableSources) > 0 {
		lines = append(lines, "Available sources: "+strings.Join(settings.AvailableSources, ", ")+".")
	}
	if strings.TrimSpace(result.Message) != "" {
		lines = append(lines, result.Message)
	}
	return strings.Join(lines, "\n") + "\nSaved worker configuration persists across controller restarts. Starting the worker requires a separate explicit request."
}

func workerStatusMessage(result backend.Result) string {
	status := result.Status
	if status.WorkerError != "" || !status.ControllerRunning {
		return result.Message
	}
	message := "Worker dispatch is stopped."
	if status.WorkerRequested {
		message = "Worker dispatch is enabled."
		if !status.WorkerRunning {
			message = "Worker dispatch is enabled, but not currently accepting work."
		}
	}
	if files.ValidID(status.ActiveJob) {
		message += " Active job: " + status.ActiveJob + "."
	}
	return message
}

// RequiresRuntimeWait identifies commands whose backend observation or service
// operation can take the lifecycle budget. Other fixed commands stay on the
// intake loop and do not wait behind a runtime mutation.
func RequiresRuntimeWait(request string) bool {
	parts := strings.Fields(request)
	if len(parts) == 0 {
		return false
	}
	return strings.EqualFold(parts[0], "/status") || strings.EqualFold(parts[0], "/controller") || strings.EqualFold(parts[0], "/worker")
}

func runtimeOperation(operation string) bool {
	return operation == "start" || operation == "stop" || operation == "restart" || operation == "status"
}

func runtimeFailure(component, operation string) string {
	if component == "controller" {
		return "The controller could not " + operation + ". Use /controller status, then /controller start if it is stopped."
	}
	return "The worker could not " + operation + ". It may be stopped or missing its task configuration. Use /worker status, then /worker start after completing local setup."
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
	runtime, runtimeKnown := detail["runtime"].(backend.RuntimeStatus)
	runtimeAvailable, _ := detail["runtime_available"].(bool)

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
	} else if runtimeKnown && runtimeAvailable && runtime.ControllerRunning {
		lines = append(lines, "Controller: running, but its status response could not be confirmed.", "Queue, active job, and worker state: unknown.")
	} else {
		if runtimeKnown && runtimeAvailable && runtime.ControllerServiceLoaded {
			lines = append(lines, "Controller: service is loaded but not running. Use /controller start.")
		} else {
			lines = append(lines, "Controller: not running or unavailable. Use /controller start.")
		}
		lines = append(lines, "Queue, active job, and worker state: unknown.")
	}
	if runtimeKnown && runtimeAvailable && runtime.ControllerRunning {
		if runtime.WorkerRequested {
			lines = append(lines, "Worker supervision: requested.")
		} else {
			lines = append(lines, "Worker supervision: not requested.")
		}
		if runtime.WorkerError != "" {
			lines = append(lines, "Worker readiness: requires attention ("+runtime.WorkerError+").")
		} else if runtime.WorkerRunning {
			lines = append(lines, "Worker readiness: accepting dispatch.")
		} else if runtime.ControllerRunning {
			lines = append(lines, "Worker readiness: not accepting dispatch.")
		} else {
			lines = append(lines, "Worker readiness: unknown because the controller is not running.")
		}
	} else {
		lines = append(lines, "Worker supervision: unknown. Use /worker status.")
		if message, ok := detail["runtime_message"].(string); ok && strings.TrimSpace(message) != "" {
			lines = append(lines, "Runtime management: "+message)
		}
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
