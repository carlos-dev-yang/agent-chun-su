// Package opsmonitor records bounded, transport-free chat flow observations.
package opsmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"chunsu/internal/chatsupervisor"
	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/service"
	"chunsu/internal/telegram"
)

const (
	Version             = 1
	Directory           = service.MonitorDirectory
	StatePath           = Directory + "/state.json"
	maxStatusTokenBytes = 64
)

type ServiceFact struct {
	Enabled           bool `json:"enabled"`
	IntentReadable    bool `json:"intent_readable"`
	RegistrationKnown bool `json:"registration_known"`
	Present           bool `json:"present"`
	StatusReadable    bool `json:"status_readable"`
	Loaded            bool `json:"loaded"`
}
type ProcessFact struct {
	Alive    bool      `json:"alive"`
	State    string    `json:"state,omitempty"`
	At       time.Time `json:"at,omitempty"`
	Restarts uint64    `json:"restarts,omitempty"`
}

type PollFact struct {
	State         string    `json:"state,omitempty"`
	ProgressKnown bool      `json:"progress_known"`
	Ready         bool      `json:"ready"`
	Waiting       bool      `json:"waiting"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	ProgressStale bool      `json:"progress_stale"`
}

type ControllerFact struct {
	Enabled         bool   `json:"enabled"`
	IntentReadable  bool   `json:"intent_readable"`
	Available       bool   `json:"available"`
	Probe           string `json:"probe,omitempty"`
	ActiveJob       string `json:"active_job,omitempty"`
	QueuePaused     bool   `json:"queue_paused,omitempty"`
	WorkerRunning   bool   `json:"worker_running,omitempty"`
	ControllerReady bool   `json:"controller_ready"`
	WorkerRequested bool   `json:"worker_requested"`
	WorkerError     string `json:"worker_error,omitempty"`
	DispatchKnown   bool   `json:"dispatch_known"`
}

type RequestFact struct {
	Active                bool      `json:"active"`
	UpdateID              int64     `json:"update_id,omitempty"`
	LastMessageObservedAt time.Time `json:"last_message_observed_at,omitempty"`
	ActiveObservedAt      time.Time `json:"active_observed_at,omitempty"`
	Stalled               bool      `json:"stalled"`
	AIBlocked             bool      `json:"ai_blocked"`
	FixedReplyObservedAt  time.Time `json:"fixed_reply_observed_at,omitempty"`
}

type ReceiptFact struct {
	WindowFiles         int       `json:"window_files"`
	Scanned             int       `json:"scanned"`
	PartialHistory      bool      `json:"partial_history"`
	Unreadable          int       `json:"unreadable"`
	Pending             int       `json:"pending"`
	Completed           int       `json:"completed"`
	Failed              int       `json:"failed"`
	Uncertain           int       `json:"uncertain"`
	Interrupted         int       `json:"interrupted"`
	ReplyUnconfirmed    int       `json:"reply_unconfirmed"`
	LastSampleOutcomeAt time.Time `json:"last_sample_outcome_at,omitempty"`
	ActiveState         string    `json:"active_state,omitempty"`
	ActiveObservedAt    time.Time `json:"active_observed_at,omitempty"`
}

// Snapshot is limited to flow facts. It contains no message text, token,
// digest, raw provider error, or worker-internal execution detail.
type Snapshot struct {
	Version        int            `json:"version"`
	ObservedAt     time.Time      `json:"observed_at"`
	ChatService    ServiceFact    `json:"chat_service"`
	MonitorService ServiceFact    `json:"monitor_service"`
	Supervisor     ProcessFact    `json:"supervisor"`
	Receiver       ProcessFact    `json:"receiver"`
	Poll           PollFact       `json:"poll"`
	Controller     ControllerFact `json:"controller"`
	// FrontendHealth reports reachability and readiness only. It does not prove
	// an AI result or remote message delivery.
	FrontendHealth      string      `json:"frontend_health"`
	BackendHealth       string      `json:"backend_health"`
	Request             RequestFact `json:"request"`
	Receipts            ReceiptFact `json:"receipts"`
	Diagnostics         []string    `json:"diagnostics,omitempty"`
	ObservationComplete bool        `json:"observation_complete"`
	Recovery            string      `json:"recovery"`
	// outcomes are used only while this observation is persisted. They are
	// deliberately not written into the public, bounded snapshot.
	outcomes map[string]diagnosticOutcome
}

type Alert struct {
	Code  string    `json:"code"`
	State string    `json:"state"`
	At    time.Time `json:"at"`
}

type diagnosticOutcome uint8

const (
	diagnosticUnknown diagnosticOutcome = iota
	diagnosticProblem
	diagnosticHealthy
	diagnosticDisabled
)

type State struct {
	Version           int                      `json:"version"`
	UpdatedAt         time.Time                `json:"updated_at"`
	Identity          platform.ProcessIdentity `json:"identity"`
	Latest            Snapshot                 `json:"latest"`
	ActiveDiagnostics []string                 `json:"active_diagnostics,omitempty"`
	Alerts            []Alert                  `json:"alerts,omitempty"`
}

type Status struct {
	Saved             bool        `json:"saved"`
	Fresh             bool        `json:"fresh"`
	ProcessAlive      bool        `json:"process_alive"`
	MonitorService    ServiceFact `json:"monitor_service"`
	Latest            Snapshot    `json:"latest,omitempty"`
	Alerts            []Alert     `json:"alerts,omitempty"`
	ActiveDiagnostics []string    `json:"active_diagnostics,omitempty"`
}

func Interval(c config.Config) time.Duration  { return telegram.HealthInterval(c.Limits) }
func freshness(c config.Config) time.Duration { return Interval(c) * telegram.MissedHealthIntervals }

func requestWindow(c config.Config) time.Duration {
	// The active receipt begins before the full AI budget; the outcome and
	// bounded reply/health work follow it.
	return time.Duration(c.Limits.TimeoutSeconds)*time.Second + telegram.StallTimeout(c.Limits)
}

func observeService(ctx context.Context, root string, c config.Config, kind string, enabled bool, intentErr error) ServiceFact {
	out := ServiceFact{Enabled: enabled, IntentReadable: intentErr == nil}
	if _, err := service.ReadFor(root, kind); errors.Is(err, os.ErrNotExist) {
		out.RegistrationKnown = true
		return out
	} else if err != nil {
		return out
	}
	out.RegistrationKnown, out.Present = true, true
	status, err := service.CommandFor(ctx, root, kind, "status", time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err == nil {
		out.StatusReadable, out.Loaded = true, status.Running
	}
	return out
}

func safeStatusToken(value string) bool {
	if value == "" || len(value) > maxStatusTokenBytes {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func observeController(ctx context.Context, root string, c config.Config, enabled bool, intentErr error) ControllerFact {
	out := ControllerFact{Enabled: enabled, IntentReadable: intentErr == nil}
	var reply struct {
		Owner           string  `json:"owner"`
		ActiveJob       *string `json:"active_job"`
		QueuePaused     *bool   `json:"queue_paused"`
		WorkerRunning   *bool   `json:"worker_running"`
		ControllerReady *bool   `json:"controller_ready"`
		WorkerRequested *bool   `json:"worker_requested"`
		WorkerError     *string `json:"worker_error"`
	}
	data, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
	if err != nil {
		out.Probe = "failed"
		return out
	}
	if !handled {
		out.Probe = "no_endpoint"
		return out
	}
	if json.Unmarshal(data, &reply) != nil || reply.Owner != "controller" || reply.ActiveJob == nil || reply.QueuePaused == nil || reply.WorkerRunning == nil {
		out.Probe = "failed"
		return out
	}
	out.Available, out.Probe, out.ActiveJob, out.QueuePaused, out.WorkerRunning = true, "responsive", *reply.ActiveJob, *reply.QueuePaused, *reply.WorkerRunning
	// The legacy status facts establish controller availability. Dispatch facts
	// are additive and are used only when the complete, bounded shape is present.
	workerError := ""
	if reply.WorkerError != nil {
		workerError = *reply.WorkerError
	}
	if reply.ControllerReady == nil || reply.WorkerRequested == nil ||
		(workerError != "" && !safeStatusToken(workerError)) {
		return out
	}
	out.DispatchKnown = true
	out.ControllerReady, out.WorkerRequested, out.WorkerError = *reply.ControllerReady, *reply.WorkerRequested, workerError
	return out
}

func observeReceipts(root string, c config.Config, activeID int64) (out ReceiptFact) {
	observeActive := func() {
		if activeID == 0 {
			return
		}
		receipt, err := telegram.ReadReceipt(root, activeID, c.Limits.MaxArtifactBytes)
		if err != nil {
			out.Unreadable++
			return
		}
		out.ActiveState, out.ActiveObservedAt = receipt.State, receipt.At
	}
	defer observeActive()
	directory := filepath.Join(telegram.Directory(root), "receipts")
	if err := files.RequirePrivateDir(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out
		}
		out.Unreadable = 1
		return out
	}
	handle, err := os.Open(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out
		}
		out.Unreadable = 1
		return out
	}
	defer handle.Close()
	entries, err := handle.ReadDir(c.Limits.MaxMessages + 1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return out
		}
		out.Unreadable = 1
		return out
	}
	if len(entries) > c.Limits.MaxMessages {
		out.PartialHistory = true
		entries = entries[:c.Limits.MaxMessages]
	}
	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if len(name) <= len(".json") {
			continue
		}
		id, err := strconv.ParseInt(name[:len(name)-len(".json")], 10, 64)
		if err == nil && telegram.ReceiptName(id) == name {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	out.WindowFiles = len(ids)
	for _, id := range ids {
		receipt, err := telegram.ReadReceipt(root, id, c.Limits.MaxArtifactBytes)
		if err != nil {
			out.Unreadable++
			continue
		}
		out.Scanned++
		terminal := true
		switch receipt.State {
		case "completed", "paired":
			out.Completed++
		case "failed", "cancelled":
			out.Failed++
		case "uncertain":
			out.Uncertain++
		case "interrupted":
			out.Interrupted++
		case "reply_unconfirmed", "pair_reply_unconfirmed":
			out.ReplyUnconfirmed++
		default:
			terminal = false
			out.Pending++
		}
		if terminal && receipt.At.After(out.LastSampleOutcomeAt) {
			// At is a receipt state observation, not a claimed reply-send time.
			out.LastSampleOutcomeAt = receipt.At
		}
	}
	return out
}

func sorted(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func problemDiagnostics(outcomes map[string]diagnosticOutcome) []string {
	values := make(map[string]bool)
	for code, outcome := range outcomes {
		if outcome == diagnosticProblem {
			values[code] = true
		}
	}
	return sorted(values)
}

func classify(outcomes map[string]diagnosticOutcome, codes, unavailable []string, disabled bool) string {
	if disabled {
		return "disabled"
	}
	unknown := false
	for _, code := range codes {
		if outcomes[code] == diagnosticUnknown {
			unknown = true
		}
	}
	for _, code := range unavailable {
		if outcomes[code] == diagnosticProblem {
			return "unavailable"
		}
	}
	for _, code := range codes {
		if outcomes[code] == diagnosticProblem {
			return "degraded"
		}
	}
	if unknown {
		return "unknown"
	}
	return "ready"
}

// Check is read-only. Poll backoff remains visible as Waiting and does not
// itself create an incident or recovery action.
func Check(ctx context.Context, root string, c config.Config) Snapshot {
	now := time.Now().UTC()
	chatEnabled, chatIntentErr := service.Enabled(root)
	monitorEnabled, monitorIntentErr := service.MonitorEnabled(root)
	controllerEnabled, controllerIntentErr := service.ControllerEnabled(root)
	out := Snapshot{
		Version: Version, ObservedAt: now,
		ChatService:    observeService(ctx, root, c, service.Chat, chatEnabled, chatIntentErr),
		MonitorService: observeService(ctx, root, c, service.Monitor, monitorEnabled, monitorIntentErr),
		Controller:     observeController(ctx, root, c, controllerEnabled, controllerIntentErr),
		Recovery:       "Inspect chunsu telegram status, chunsu controller status, and chunsu errors; manually start only the component you choose.",
		outcomes:       make(map[string]diagnosticOutcome),
	}
	frontendCodes := []string{"chat_service_state_unreadable", "chat_service_unregistered", "chat_service_status_unreadable", "supervisor_state_unreadable", "supervisor_unavailable", "receiver_state_unreadable", "receiver_unavailable", "ai_recovery_required", "poll_progress_stalled", "active_request_stalled"}
	backendCodes := []string{"controller_intent_unreadable", "controller_unavailable", "controller_probe_failed", "backend_dispatch_status_unreadable", "backend_dispatch_unavailable", "backend_dispatch_error"}
	supervisor, supervisorAlive, supervisorErr := chatsupervisor.Read(root, c.Limits)
	out.Supervisor = ProcessFact{Alive: supervisorAlive, State: supervisor.State, At: supervisor.At, Restarts: supervisor.Restarts}
	health, receiverAlive, receiverErr := telegram.ReadHealth(root, c.Limits)
	out.Receipts = observeReceipts(root, c, health.ActiveUpdate)
	out.Receiver = ProcessFact{Alive: receiverAlive, State: health.State, At: health.At}
	pollProgressKnown := receiverErr == nil && receiverAlive && health.Poll == "connected" && !health.LastPollAt.IsZero() && !health.LastPollAt.After(now)
	out.Poll = PollFact{State: health.Poll, ProgressKnown: pollProgressKnown, Ready: pollProgressKnown && now.Sub(health.LastPollAt) <= telegram.StallTimeout(c.Limits), Waiting: receiverErr == nil && receiverAlive && health.Poll != "connected", LastSuccessAt: health.LastPollAt}
	out.Poll.ProgressStale = pollProgressKnown && now.Sub(health.LastPollAt) > telegram.StallTimeout(c.Limits)
	if out.Poll.ProgressStale {
		out.Poll.Ready = false
	}
	out.Request = RequestFact{Active: health.ActiveUpdate != 0, UpdateID: health.ActiveUpdate, LastMessageObservedAt: health.LastMessageAt, ActiveObservedAt: out.Receipts.ActiveObservedAt, AIBlocked: health.AIBlocked, FixedReplyObservedAt: health.LastReplyAt}
	if out.Request.Active && !out.Request.ActiveObservedAt.IsZero() {
		out.Request.Stalled = now.Sub(out.Request.ActiveObservedAt) > requestWindow(c)
	}
	outcomes := out.outcomes
	if chatIntentErr != nil {
		outcomes["chat_intent_unreadable"] = diagnosticProblem
		for _, code := range frontendCodes {
			outcomes[code] = diagnosticUnknown
		}
	} else {
		outcomes["chat_intent_unreadable"] = diagnosticHealthy
		if !chatEnabled {
			for _, code := range frontendCodes {
				outcomes[code] = diagnosticDisabled
			}
		} else {
			if !out.ChatService.RegistrationKnown {
				outcomes["chat_service_state_unreadable"], outcomes["chat_service_unregistered"], outcomes["chat_service_status_unreadable"] = diagnosticProblem, diagnosticUnknown, diagnosticUnknown
			} else if !out.ChatService.Present {
				outcomes["chat_service_state_unreadable"], outcomes["chat_service_unregistered"], outcomes["chat_service_status_unreadable"] = diagnosticHealthy, diagnosticProblem, diagnosticUnknown
			} else {
				outcomes["chat_service_state_unreadable"], outcomes["chat_service_unregistered"] = diagnosticHealthy, diagnosticHealthy
				if out.ChatService.StatusReadable {
					outcomes["chat_service_status_unreadable"] = diagnosticHealthy
				} else {
					outcomes["chat_service_status_unreadable"] = diagnosticProblem
				}
			}
			if supervisorErr != nil {
				outcomes["supervisor_state_unreadable"], outcomes["supervisor_unavailable"] = diagnosticProblem, diagnosticUnknown
			} else {
				outcomes["supervisor_state_unreadable"] = diagnosticHealthy
				if supervisorAlive {
					outcomes["supervisor_unavailable"] = diagnosticHealthy
				} else {
					outcomes["supervisor_unavailable"] = diagnosticProblem
				}
			}
			if receiverErr != nil {
				outcomes["receiver_state_unreadable"], outcomes["receiver_unavailable"] = diagnosticProblem, diagnosticUnknown
			} else {
				outcomes["receiver_state_unreadable"] = diagnosticHealthy
				if receiverAlive {
					outcomes["receiver_unavailable"] = diagnosticHealthy
				} else {
					outcomes["receiver_unavailable"] = diagnosticProblem
				}
			}
			if receiverErr != nil || !receiverAlive {
				outcomes["ai_recovery_required"], outcomes["poll_progress_stalled"], outcomes["active_request_stalled"] = diagnosticUnknown, diagnosticUnknown, diagnosticUnknown
			} else {
				if health.AIBlocked {
					outcomes["ai_recovery_required"] = diagnosticProblem
				} else {
					outcomes["ai_recovery_required"] = diagnosticHealthy
				}
				if pollProgressKnown {
					if out.Poll.ProgressStale {
						outcomes["poll_progress_stalled"] = diagnosticProblem
					} else {
						outcomes["poll_progress_stalled"] = diagnosticHealthy
					}
				} else {
					outcomes["poll_progress_stalled"] = diagnosticUnknown
				}
				if !out.Request.Active {
					outcomes["active_request_stalled"] = diagnosticHealthy
				} else if out.Request.ActiveObservedAt.IsZero() || out.Request.ActiveObservedAt.After(now) {
					outcomes["active_request_stalled"] = diagnosticUnknown
				} else if out.Request.Stalled {
					outcomes["active_request_stalled"] = diagnosticProblem
				} else {
					outcomes["active_request_stalled"] = diagnosticHealthy
				}
			}
		}
	}
	if controllerIntentErr != nil {
		outcomes["controller_intent_unreadable"] = diagnosticProblem
		for _, code := range backendCodes[1:] {
			outcomes[code] = diagnosticUnknown
		}
	} else if !controllerEnabled {
		outcomes["controller_intent_unreadable"] = diagnosticHealthy
		for _, code := range backendCodes[1:] {
			outcomes[code] = diagnosticDisabled
		}
	} else {
		outcomes["controller_intent_unreadable"] = diagnosticHealthy
		switch out.Controller.Probe {
		case "responsive":
			outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticHealthy, diagnosticHealthy
			if !out.Controller.DispatchKnown {
				outcomes["backend_dispatch_status_unreadable"], outcomes["backend_dispatch_unavailable"], outcomes["backend_dispatch_error"] = diagnosticUnknown, diagnosticUnknown, diagnosticUnknown
			} else {
				outcomes["backend_dispatch_status_unreadable"] = diagnosticHealthy
				if !out.Controller.ControllerReady || out.Controller.WorkerRequested && !out.Controller.WorkerRunning && out.Controller.WorkerError == "" {
					outcomes["backend_dispatch_unavailable"] = diagnosticProblem
				} else {
					outcomes["backend_dispatch_unavailable"] = diagnosticHealthy
				}
				if out.Controller.WorkerError != "" {
					outcomes["backend_dispatch_error"] = diagnosticProblem
				} else {
					outcomes["backend_dispatch_error"] = diagnosticHealthy
				}
			}
		case "no_endpoint":
			outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticProblem, diagnosticHealthy
			outcomes["backend_dispatch_status_unreadable"], outcomes["backend_dispatch_unavailable"], outcomes["backend_dispatch_error"] = diagnosticUnknown, diagnosticUnknown, diagnosticUnknown
		case "failed":
			outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticUnknown, diagnosticProblem
			outcomes["backend_dispatch_status_unreadable"], outcomes["backend_dispatch_unavailable"], outcomes["backend_dispatch_error"] = diagnosticUnknown, diagnosticUnknown, diagnosticUnknown
		default:
			for _, code := range backendCodes[1:] {
				outcomes[code] = diagnosticUnknown
			}
		}
	}
	out.FrontendHealth = classify(outcomes, frontendCodes, []string{"chat_service_unregistered", "supervisor_unavailable", "receiver_unavailable"}, chatIntentErr == nil && !chatEnabled)
	out.BackendHealth = classify(outcomes, backendCodes, []string{"controller_unavailable", "backend_dispatch_unavailable"}, controllerIntentErr == nil && !controllerEnabled)
	out.Diagnostics = problemDiagnostics(outcomes)
	out.ObservationComplete = true
	for _, outcome := range outcomes {
		if outcome == diagnosticUnknown {
			out.ObservationComplete = false
			break
		}
	}
	return out
}

func read(root string, c config.Config) (State, bool, error) {
	var state State
	data, err := files.Read(root, StatePath, c.Limits.MaxArtifactBytes)
	if errors.Is(err, os.ErrNotExist) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	if err = json.Unmarshal(data, &state); err != nil || state.Version != Version {
		return state, false, errors.New("invalid monitor state")
	}
	return state, true, nil
}

func ReadStatus(ctx context.Context, root string, c config.Config) (Status, error) {
	state, saved, err := read(root, c)
	if err != nil {
		return Status{}, err
	}
	enabled, intentErr := service.MonitorEnabled(root)
	out := Status{Saved: saved, MonitorService: observeService(ctx, root, c, service.Monitor, enabled, intentErr)}
	if saved {
		out.Latest, out.Alerts, out.ActiveDiagnostics = state.Latest, state.Alerts, state.ActiveDiagnostics
		actual, identityErr := platform.Identify(state.Identity.PID)
		out.ProcessAlive = identityErr == nil && actual == state.Identity
		out.Fresh = out.ProcessAlive && !state.UpdatedAt.IsZero() && time.Since(state.UpdatedAt) >= 0 && time.Since(state.UpdatedAt) < freshness(c)
	}
	return out, nil
}

func persist(root string, c config.Config, identity platform.ProcessIdentity, snapshot Snapshot) error {
	state, _, err := read(root, c)
	if err != nil {
		return err
	}
	previous, current, known := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, code := range state.ActiveDiagnostics {
		previous[code] = true
	}
	for code, outcome := range snapshot.outcomes {
		switch outcome {
		case diagnosticProblem:
			current[code], known[code] = true, true
		case diagnosticHealthy:
			known[code] = true
		}
	}
	for _, code := range sorted(known) {
		if current[code] && !previous[code] {
			state.Alerts = append(state.Alerts, Alert{Code: code, State: "raised", At: snapshot.ObservedAt})
		}
		if !current[code] && previous[code] {
			state.Alerts = append(state.Alerts, Alert{Code: code, State: "resolved", At: snapshot.ObservedAt})
		}
		if current[code] {
			previous[code] = true
		} else {
			delete(previous, code)
		}
	}
	state.ActiveDiagnostics = sorted(previous)
	if len(state.Alerts) > c.Limits.MaxMessages {
		state.Alerts = append([]Alert(nil), state.Alerts[len(state.Alerts)-c.Limits.MaxMessages:]...)
	}
	state.Version, state.UpdatedAt, state.Identity, state.Latest = Version, snapshot.ObservedAt, identity, snapshot
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(root, StatePath, append(data, '\n'), true)
}

// Run holds its private monitor lock for the foreground lifetime. It never
// takes the SQLite writer lock, reads credentials, sends a message, or
// restarts a chat process.
func Run(ctx context.Context, root string, c config.Config, once, managed bool) (runErr error) {
	if managed {
		enabled, err := service.MonitorEnabled(root)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
	}
	defer func() {
		if !managed || ctx.Err() == nil || runErr != nil {
			return
		}
		if enabled, err := service.MonitorEnabled(root); err == nil && enabled {
			runErr = errors.New("monitor interrupted while enabled")
		}
	}()
	identity, err := platform.Identify(os.Getpid())
	if err != nil {
		return err
	}
	lock, err := acquire(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	if managed {
		enabled, err := service.MonitorEnabled(root)
		if err != nil {
			return err
		}
		if !enabled {
			return nil
		}
	}
	observe := func() error { return persist(root, c, identity, Check(ctx, root, c)) }
	if err := observe(); err != nil || once {
		return err
	}
	ticker := time.NewTicker(Interval(c))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if managed {
				enabled, err := service.MonitorEnabled(root)
				if err != nil {
					return err
				}
				if !enabled {
					return nil
				}
			}
			if err := observe(); err != nil {
				return err
			}
		}
	}
}
