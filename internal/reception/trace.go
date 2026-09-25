package reception

import (
	"encoding/json"
	"errors"
	"time"

	"chunsu/internal/conversation"
	"chunsu/internal/errorreport"
	"chunsu/internal/features"
	"chunsu/internal/files"
)

type EventKind string

const (
	EventThinking EventKind = "thinking"
	EventProgress EventKind = "progress"
	EventReply    EventKind = "reply"
	EventAction   EventKind = "action"

	receptionArtifactVersion = 1
	audienceInternal         = "internal"
	audienceUser             = "user"
	deliverySuppressed       = "suppressed"
	deliverySending          = "sending"
	deliverySent             = "sent"
	deliveryUnconfirmed      = "unconfirmed"
)

// UserVisible is host-owned; new or unrecognized kinds stay internal by default.
func (e Event) UserVisible() bool { return e.Kind == EventReply }

type TraceContext struct {
	Version   int       `json:"version"`
	Time      time.Time `json:"time"`
	SessionID string    `json:"session_id"`
	RequestID string    `json:"request_id"`
	Step      int       `json:"step"`
	UpdateID  *int64    `json:"update_id,omitempty"`
}

type actionRecord struct {
	TraceContext
	EventKind          EventKind           `json:"event_kind"`
	Audience           string              `json:"audience"`
	Delivery           string              `json:"delivery"`
	Action             conversation.Action `json:"action"`
	InputDigest        string              `json:"input_digest,omitempty"`
	State              string              `json:"state"`
	ResultDigest       string              `json:"result_digest,omitempty"`
	StoredResultDigest string              `json:"stored_result_digest,omitempty"`
	Result             *HostResult         `json:"result,omitempty"`
}

type messageRecord struct {
	TraceContext
	EventKind  EventKind `json:"event_kind"`
	Audience   string    `json:"audience"`
	TextDigest string    `json:"text_digest"`
	Delivery   string    `json:"delivery"`
}

func (s *Session) traceContext(requestID string, step int) TraceContext {
	trace := TraceContext{Version: receptionArtifactVersion, Time: time.Now().UTC(), SessionID: s.ID, RequestID: requestID, Step: step}
	if s.Channel == Telegram && s.UpdateID >= 0 {
		id := s.UpdateID
		trace.UpdateID = &id
	}
	return trace
}

func (s *Session) writeJSON(directory, name string, value any, replace bool) error {
	if err := files.PrivateDir(directory); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if int64(len(data)) > s.Config.Limits.MaxArtifactBytes {
		return errors.New("reception trace exceeds configured artifact limit")
	}
	return files.Write(directory, name, data, replace)
}

func (s *Session) deliver(directory, requestID string, step int, event Event, emit func(Event) error) error {
	record := messageRecord{TraceContext: s.traceContext(requestID, step), EventKind: event.Kind, TextDigest: files.Digest([]byte(event.Text))}
	if !event.UserVisible() {
		record.Audience, record.Delivery = audienceInternal, deliverySuppressed
		return s.writeJSON(directory, "message.json", record, false)
	}
	record.Audience, record.Delivery = audienceUser, deliverySending
	if err := s.writeJSON(directory, "message.json", record, false); err != nil {
		return err
	}
	err := emit(event)
	record.Time = time.Now().UTC()
	record.Delivery = deliverySent
	if err != nil {
		record.Delivery = deliveryUnconfirmed
	}
	saveErr := s.writeJSON(directory, "message.json", record, true)
	if err != nil || saveErr != nil {
		return errors.Join(ErrUncertain, err, saveErr)
	}
	return nil
}

// Only host-owned projections are retained. Local callback payloads and new
// actions default to status only; model history continues to use the original result.
func traceResult(action string, result HostResult) HostResult {
	trace := HostResult{Status: result.Status}
	switch action {
	case conversation.WebOpen:
		trace.Detail = traceMap(result.Detail, "url", "sha256", "retrieved_at", "truncated")
	case conversation.WebSearch:
		trace.Detail = traceMap(result.Detail, "provider", "retrieved_at")
	case conversation.ReadGuide, conversation.AcknowledgeError:
		if detail, ok := result.Detail.(string); ok {
			trace.Detail = detail
		}
	case conversation.ListFeatures:
		if detail, ok := result.Detail.([]features.Feature); ok {
			trace.Detail = detail
		}
	case conversation.ListErrors:
		if detail, ok := result.Detail.([]errorreport.Report); ok {
			trace.Detail = detail
		}
	case conversation.RuntimeStatus:
		trace.Detail = traceMap(result.Detail, "controller_available", "controller_status_readable", "controller", "runtime", "runtime_available", "runtime_message", "chat_receiver_alive", "chat_health_readable", "chat", "recovery")
	case conversation.ListJobs:
		trace.Detail = traceMap(result.Detail, "jobs", "total")
	case conversation.InstallFeature:
		trace.Detail = traceMap(result.Detail, "feature", "status", "active_digest", "pack_digest", "next")
	case conversation.InstallGuide:
		trace.Detail = traceMap(result.Detail, "service", "pack_digest", "message")
	case conversation.StartWorker, conversation.StopWorker, conversation.RestartWorker:
		trace.Detail = traceMap(result.Detail, "message")
	case conversation.ReadWorkerConfig, conversation.ConfigureWorker:
		trace.Detail = traceMap(result.Detail, "message", "settings")
	case conversation.PauseQueue, conversation.ResumeQueue:
		trace.Detail = traceMap(result.Detail, "queue_paused", "active_job", "active_work")
	case conversation.CancelJob, conversation.RetryJob:
		trace.Detail = traceMap(result.Detail, "job_id", "status")
	case conversation.DelegateJob:
		trace.Detail = traceMap(result.Detail, "job_id", "workgroup", "state", "note")
	}
	return trace
}

func traceAction(action conversation.Action) (conversation.Action, string) {
	if action.Name != conversation.WebSearch && action.Name != conversation.WebOpen {
		return action, ""
	}
	digest := files.Digest([]byte(action.Reference))
	action.Reference = ""
	return action, digest
}

func traceMap(detail any, keys ...string) any {
	raw, err := json.Marshal(detail)
	if err != nil {
		return nil
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	trace := make(map[string]json.RawMessage, len(keys))
	for _, key := range keys {
		if value, ok := values[key]; ok {
			trace[key] = value
		}
	}
	if len(trace) == 0 {
		return nil
	}
	return trace
}
