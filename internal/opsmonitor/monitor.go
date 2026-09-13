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
	Version   = 1
	Directory = service.MonitorDirectory
	StatePath = Directory + "/state.json"
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
	Available     bool   `json:"available"`
	Probe         string `json:"probe,omitempty"`
	ActiveJob     string `json:"active_job,omitempty"`
	QueuePaused   bool   `json:"queue_paused,omitempty"`
	WorkerRunning bool   `json:"worker_running,omitempty"`
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
	Version             int            `json:"version"`
	ObservedAt          time.Time      `json:"observed_at"`
	ChatService         ServiceFact    `json:"chat_service"`
	MonitorService      ServiceFact    `json:"monitor_service"`
	Supervisor          ProcessFact    `json:"supervisor"`
	Receiver            ProcessFact    `json:"receiver"`
	Poll                PollFact       `json:"poll"`
	Controller          ControllerFact `json:"controller"`
	Request             RequestFact    `json:"request"`
	Receipts            ReceiptFact    `json:"receipts"`
	Diagnostics         []string       `json:"diagnostics,omitempty"`
	ObservationComplete bool           `json:"observation_complete"`
	Recovery            string         `json:"recovery"`
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

func observeController(ctx context.Context, root string, c config.Config) ControllerFact {
	var reply struct {
		Owner         string `json:"owner"`
		ActiveJob     string `json:"active_job"`
		QueuePaused   *bool  `json:"queue_paused"`
		WorkerRunning *bool  `json:"worker_running"`
	}
	data, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
	if err != nil {
		return ControllerFact{Probe: "failed"}
	}
	if !handled {
		return ControllerFact{Probe: "no_endpoint"}
	}
	if json.Unmarshal(data, &reply) != nil || reply.Owner != "controller" || reply.QueuePaused == nil || reply.WorkerRunning == nil {
		return ControllerFact{Probe: "failed"}
	}
	return ControllerFact{Available: true, Probe: "responsive", ActiveJob: reply.ActiveJob, QueuePaused: *reply.QueuePaused, WorkerRunning: *reply.WorkerRunning}
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

// Check is read-only. Poll backoff remains visible as Waiting and does not
// itself create an incident or recovery action.
func Check(ctx context.Context, root string, c config.Config) Snapshot {
	now := time.Now().UTC()
	chatEnabled, chatIntentErr := service.Enabled(root)
	monitorEnabled, monitorIntentErr := service.MonitorEnabled(root)
	out := Snapshot{
		Version: Version, ObservedAt: now,
		ChatService:    observeService(ctx, root, c, service.Chat, chatEnabled, chatIntentErr),
		MonitorService: observeService(ctx, root, c, service.Monitor, monitorEnabled, monitorIntentErr),
		Controller:     observeController(ctx, root, c),
		Recovery:       "Inspect chunsu telegram status and chunsu errors; manually start or restart only the component you choose.",
		outcomes:       make(map[string]diagnosticOutcome),
	}
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
		for _, code := range []string{"chat_service_state_unreadable", "chat_service_unregistered", "chat_service_status_unreadable", "supervisor_state_unreadable", "supervisor_unavailable", "receiver_state_unreadable", "receiver_unavailable", "ai_recovery_required", "controller_unavailable", "controller_probe_failed", "poll_progress_stalled", "active_request_stalled"} {
			outcomes[code] = diagnosticUnknown
		}
	} else {
		outcomes["chat_intent_unreadable"] = diagnosticHealthy
		if !chatEnabled {
			for _, code := range []string{"chat_service_state_unreadable", "chat_service_unregistered", "chat_service_status_unreadable", "supervisor_state_unreadable", "supervisor_unavailable", "receiver_state_unreadable", "receiver_unavailable", "ai_recovery_required", "controller_unavailable", "controller_probe_failed", "poll_progress_stalled", "active_request_stalled"} {
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
			switch out.Controller.Probe {
			case "responsive":
				outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticHealthy, diagnosticHealthy
			case "no_endpoint":
				outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticProblem, diagnosticHealthy
			case "failed":
				outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticUnknown, diagnosticProblem
			default:
				outcomes["controller_unavailable"], outcomes["controller_probe_failed"] = diagnosticUnknown, diagnosticUnknown
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
