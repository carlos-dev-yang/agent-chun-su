package reception

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"chunsu/internal/backend"
	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/features"
	"chunsu/internal/onboarding"
	"chunsu/internal/store"
	"chunsu/internal/telegram"
	"chunsu/internal/updateguard"
	"chunsu/internal/workerconfig"
)

type HostResult struct {
	Status string `json:"status"`
	Detail any    `json:"detail,omitempty"`
}

// controllerStatus contains only a controller response that passed the IPC
// shape checks. A zero value is deliberately not treated as an observation.
type controllerStatus struct {
	ActiveJob     string `json:"active_job"`
	Owner         string `json:"owner"`
	QueuePaused   bool   `json:"queue_paused"`
	WorkerRunning bool   `json:"worker_running"`
}

type controllerStatusReply struct {
	ActiveJob     *string `json:"active_job"`
	Owner         string  `json:"owner"`
	QueuePaused   *bool   `json:"queue_paused"`
	WorkerRunning *bool   `json:"worker_running"`
}

// Local callbacks contain user interaction only. Common admission, authority
// checks and result projection do not depend on Cobra or a particular UI.
type Host struct {
	Root           string
	Config         config.Config
	GmailSetup     func(context.Context) (HostResult, error)
	DisplayReport  func(context.Context, []byte) error
	GuideInstalled func(onboarding.Result)
}

// workerRuntimeError carries only the backend's fixed worker message. Its cause
// stays available for classification and diagnostics, but is never projected
// into conversation history or a user-facing failure.
type workerRuntimeError struct {
	message string
	cause   error
}

func (e *workerRuntimeError) Error() string { return "worker runtime action failed" }
func (e *workerRuntimeError) Unwrap() error { return e.cause }

func (h Host) call(ctx context.Context, request control.Request, result any) error {
	data, handled, err := control.Call(ctx, h.Root, time.Duration(h.Config.Limits.LockWaitSeconds)*time.Second, h.Config.Limits.MaxArtifactBytes, request)
	if err != nil {
		return err
	}
	if !handled {
		return errors.New("host controller is not running")
	}
	return json.Unmarshal(data, result)
}

func (h Host) runtime(ctx context.Context, component, operation string) (backend.Result, error) {
	return backend.Command(ctx, h.Root, h.Config, component, operation)
}

func (h Host) workerConfig(ctx context.Context, operation, value string) (workerconfig.Result, error) {
	return backend.WorkerConfig(ctx, h.Root, h.Config, operation, value)
}

func (h Host) Dispatch(ctx context.Context, channel string, action conversation.Action, request string, known map[string]bool) (HostResult, error) {
	if action.Name != conversation.RuntimeStatus && action.Name != conversation.ListErrors && action.Name != conversation.ReadGuide {
		active, err := updateguard.Active(h.Root, h.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return HostResult{}, err
		}
		if active {
			return HostResult{}, errors.New("self-update activation is in progress")
		}
	}
	if err := conversation.Validate(conversation.Reply{Message: "dispatch", Action: action}); err != nil {
		return HostResult{}, err
	}
	if !allowed(channel, action.Name) {
		return HostResult{}, errors.New("reception channel does not permit this action")
	}
	switch action.Name {
	case conversation.StartWorker, conversation.StopWorker, conversation.RestartWorker:
		operation := "start"
		if action.Name == conversation.StopWorker {
			operation = "stop"
		} else if action.Name == conversation.RestartWorker {
			operation = "restart"
		}
		result, err := h.runtime(ctx, "worker", operation)
		outcome := HostResult{Status: "processed", Detail: map[string]string{"message": result.Message}}
		if err != nil {
			outcome.Status = "failed"
			return outcome, &workerRuntimeError{message: result.Message, cause: err}
		}
		return outcome, nil
	case conversation.ReadWorkerConfig, conversation.ConfigureWorker:
		operation, value := "status", ""
		if action.Name == conversation.ConfigureWorker {
			if action.Service == "model" && !strings.Contains(request, action.Reference) {
				return HostResult{}, errors.New("changing the worker model requires the model stated by the user")
			}
			operation, value = action.Service, action.Reference
		}
		result, err := h.workerConfig(ctx, operation, value)
		outcome := HostResult{Status: "observed", Detail: result}
		if action.Name == conversation.ConfigureWorker {
			outcome.Status = "processed"
		}
		if err != nil {
			outcome.Status = "failed"
			return outcome, &workerRuntimeError{message: result.Message, cause: err}
		}
		return outcome, nil
	case conversation.ListFeatures:
		items, err := features.Catalog()
		return HostResult{Status: "available", Detail: items}, err
	case conversation.InstallFeature:
		var result any
		err := h.call(ctx, control.Request{Operation: "install_feature", SourceName: action.Service}, &result)
		return HostResult{Status: "processed", Detail: result}, err
	case conversation.ListErrors:
		items, err := errorreport.List(h.Root, h.Config.Limits)
		return HostResult{Status: "observed", Detail: items}, err
	case conversation.AcknowledgeError:
		if !strings.Contains(request, action.Reference) {
			return HostResult{}, errors.New("acknowledging an error requires the error ID supplied by the user")
		}
		err := errorreport.Acknowledge(ctx, h.Root, action.Reference, h.Config.Limits)
		return HostResult{Status: "acknowledged", Detail: "The current occurrences were acknowledged. The record is retained."}, err
	case conversation.PauseQueue, conversation.ResumeQueue:
		operation := "pause"
		if action.Name == conversation.ResumeQueue {
			operation = "unpause"
		}
		var result any
		err := h.call(ctx, control.Request{Operation: operation}, &result)
		return HostResult{Status: "processed", Detail: result}, err
	case conversation.CancelJob, conversation.RetryJob:
		if !known[action.Reference] && !strings.Contains(request, action.Reference) {
			return HostResult{}, errors.New("this action requires a job ID supplied by the user or shown in the current job list")
		}
		operation := "cancel"
		if action.Name == conversation.RetryJob {
			operation = "retry"
		}
		var result any
		err := h.call(ctx, control.Request{Operation: operation, JobID: action.Reference}, &result)
		return HostResult{Status: "processed", Detail: result}, err
	case conversation.RuntimeStatus:
		var reply controllerStatusReply
		controllerErr := h.call(ctx, control.Request{Operation: "status"}, &reply)
		controllerAvailable := controllerErr == nil && reply.Owner == "controller" && reply.ActiveJob != nil && reply.QueuePaused != nil && reply.WorkerRunning != nil
		var status controllerStatus
		if controllerAvailable {
			status = controllerStatus{ActiveJob: *reply.ActiveJob, Owner: reply.Owner, QueuePaused: *reply.QueuePaused, WorkerRunning: *reply.WorkerRunning}
		}
		health, alive, healthErr := telegram.ReadHealth(h.Root, h.Config.Limits)
		result := map[string]any{
			"controller_available":       controllerAvailable,
			"controller_status_readable": controllerAvailable,
			"controller":                 status,
			"chat_receiver_alive":        alive,
			"chat_health_readable":       healthErr == nil,
		}
		managed, runtimeErr := h.runtime(ctx, "controller", "status")
		result["runtime"] = managed.Status
		result["runtime_available"] = runtimeErr == nil
		result["runtime_message"] = managed.Message
		if strings.TrimSpace(managed.Message) == "" && runtimeErr != nil {
			result["runtime_message"] = "Runtime management is unavailable. /controller start can recover a configured controller."
		}
		if healthErr == nil {
			result["chat"] = health
		}
		if !controllerAvailable {
			result["recovery"] = "The controller did not answer its status request. Use /controller start to recover it."
		}
		return HostResult{Status: "observed", Detail: result}, runtimeErr
	case conversation.ReadGuide:
		b, err := conversation.Guide(action.Service)
		if err != nil {
			return HostResult{}, err
		}
		if int64(len(b)) > h.Config.Limits.MaxSourceBytes {
			return HostResult{}, errors.New("guide exceeds the conversation input limit")
		}
		return HostResult{Status: "guide_read", Detail: string(b)}, nil
	case conversation.InstallGuide:
		b, _ := json.Marshal(onboarding.Request{Service: action.Service})
		var result onboarding.Result
		if err := h.call(ctx, control.Request{Operation: onboarding.Prefix + "prepare", Input: b}, &result); err != nil {
			return HostResult{}, err
		}
		if channel == Local && h.GuideInstalled != nil {
			h.GuideInstalled(result)
		}
		return HostResult{Status: result.Status, Detail: map[string]string{"service": action.Service, "pack_digest": result.PackDigest, "message": result.Message}}, nil
	case conversation.GmailSetup:
		if channel != Local || h.GmailSetup == nil {
			return HostResult{}, errors.New("Gmail authentication must be completed in the local input screen")
		}
		return h.GmailSetup(ctx)
	case conversation.ListJobs:
		s, err := store.OpenReadOnly(ctx, h.Root)
		if err != nil {
			return HostResult{}, err
		}
		defer s.Close()
		jobs, err := s.Jobs(ctx)
		if err != nil {
			return HostResult{}, err
		}
		type summary struct {
			ID        string `json:"id"`
			Workgroup string `json:"workgroup"`
			Status    string `json:"status"`
			CreatedAt int64  `json:"created_at"`
		}
		items := []summary{}
		for i, job := range jobs {
			if i >= h.Config.Limits.MaxMessages {
				break
			}
			items = append(items, summary{job.ID, job.Workgroup, job.Status, job.CreatedAt})
			known[job.ID] = true
		}
		return HostResult{Status: "observed", Detail: map[string]any{"jobs": items, "total": len(jobs)}}, nil
	case conversation.DelegateJob, conversation.ShowReport:
		if !known[action.Reference] && !strings.Contains(request, action.Reference) {
			return HostResult{}, errors.New("select a job ID from the current job list or provide one directly")
		}
		if action.Name == conversation.DelegateJob {
			var job store.Job
			if err := h.call(ctx, control.Request{Operation: "delegate", JobID: action.Reference, Answer: request}, &job); err != nil {
				return HostResult{}, err
			}
			known[job.ID] = true
			return HostResult{Status: "queued", Detail: map[string]string{"job_id": job.ID, "workgroup": job.Workgroup, "state": job.Status, "note": "The request was accepted as separate work. A running worker will process it; no result has been generated yet."}}, nil
		}
		if channel != Local || h.DisplayReport == nil {
			return HostResult{}, errors.New("this channel cannot display report bodies")
		}
		s, err := store.OpenReadOnly(ctx, h.Root)
		if err != nil {
			return HostResult{}, err
		}
		defer s.Close()
		job, err := s.Job(ctx, action.Reference)
		if err != nil {
			return HostResult{}, err
		}
		artifacts, err := s.Artifacts(ctx, job.ID)
		if err != nil {
			return HostResult{}, err
		}
		for i := len(artifacts) - 1; i >= 0; i-- {
			artifact := artifacts[i]
			if artifact.Kind != "report_markdown" || artifact.AttemptID != job.CurrentAttempt {
				continue
			}
			b, err := s.ReadArtifact(artifact, h.Config.Limits.MaxArtifactBytes)
			if err != nil {
				return HostResult{}, err
			}
			if err = h.DisplayReport(ctx, b); err != nil {
				return HostResult{}, err
			}
			return HostResult{Status: "displayed_locally", Detail: "The verified retained report was shown locally. Its body was not sent to the reception AI; do not claim it was read or summarized."}, nil
		}
		return HostResult{}, errors.New("no retained report is available")
	default:
		return HostResult{}, errors.New("unsupported reception action")
	}
}
