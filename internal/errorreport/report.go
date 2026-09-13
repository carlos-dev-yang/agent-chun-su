// Package errorreport stores bounded operational diagnostics independently of
// the SQLite controller. Only catalogued messages and opaque correlations cross
// this boundary; error strings and conversation contents are never accepted.
package errorreport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/platform"
)

const Directory = "state/errors"
const Version = 1

type Diagnostic struct {
	Component string `json:"component"`
	Summary   string `json:"summary"`
	Recovery  string `json:"recovery"`
}

var catalog = map[string]Diagnostic{
	"supervisor_recovered":      {"supervisor", "Interrupted chat supervisor recovered", "The OS service manager restarted the supervisor. It will inspect the prior receiver and work before reconnecting."},
	"supervisor_interrupted":    {"supervisor", "Chat supervisor received a stop signal while enabled", "The OS service manager will recover it. Use chunsu telegram stop for a persistent stop."},
	"model_timeout":             {"reception", "AI reply timed out", "The current request was stopped. Use /reset to clear context or send a smaller request."},
	"model_not_configured":      {"reception", "Conversation AI executor is not configured", "Check chunsu config route reception and chunsu doctor on the host. Fixed commands remain available."},
	"model_compatibility":       {"reception", "Conversation AI version or model is incompatible", "Use chunsu doctor on the host to check supported versions for the conversation route and configure an executor."},
	"chat_style_unavailable":    {"reception", "Saved tone instruction is unavailable", "Run /tone reset, then check the data directory permissions and available storage on the host."},
	"chat_language_unavailable": {"reception", "Saved chat language preference is unavailable", "Run /language reset, then check the private preference storage permissions and available space."},
	"secret_unavailable":        {"chat", "Chat secret store or bot token is unavailable", "Restore the host Keychain or CHUNSU_SECRET_HELPER configuration and the saved bot token. Do not send tokens in chat."},
	"poll_failed":               {"telegram", "Message polling connection failed", "The receiver reconnects from its saved position. If this repeats, check the host network."},
	"authentication_failed":     {"telegram", "Telegram authentication or access was denied", "Polling retries continue. Restore the bot token and access permissions on the host."},
	"poll_conflict":             {"telegram", "Another receiver or webhook conflicts with polling", "Check other receivers or webhooks using this bot. Existing webhooks are not removed automatically."},
	"send_unconfirmed":          {"telegram", "Reply or acknowledgement delivery was not confirmed", "It will not be resent automatically to avoid duplicates. Check /status or /jobs for the result."},
	"receipt_failed":            {"telegram", "Message processing state could not be saved", "This request is held. Check the host data directory permissions and available storage."},
	"model_failed":              {"reception", "AI reply generation failed", "The receiver and fixed commands remain available. Use /reset or chunsu doctor on the host to check the executor and authentication."},
	"execution_uncertain":       {"reception", "Execution completion or job result needs inspection", "It will not be run again automatically. Check /jobs and the host recovery state."},
	"host_failed":               {"controller", "Internal management action failed", "The receiver remains available. Check /status and /errors, then recover the host if needed."},
	"turn_panicked":             {"reception", "Internal failure while processing the conversation", "That conversation was stopped. Use the error ID and installed version when inspecting it."},
	"startup_failed":            {"chat", "Chat receiver failed to start", "The supervisor will retry. Check configuration, the secret store, and whether another receiver is running."},
	"receiver_exited":           {"supervisor", "Chat receiver exited unexpectedly", "The supervisor will restart the receiver. Interrupted requests are not run again automatically."},
	"receiver_stalled":          {"supervisor", "Chat receiver response check timed out", "Only the supervisor-owned receiver will be cleaned up and restarted."},
	"recovery_blocked":          {"supervisor", "Safe termination of a prior run could not be confirmed", "Duplicate execution is held. Inspect remaining processes and execution records on the host."},
	"supervisor_state_failed":   {"supervisor", "Supervisor state could not be saved", "Restore the host data directory permissions and available storage."},
	"interrupted_request":       {"reception", "A request unfinished before restart was found", "Its result may be uncertain, so it will not run again automatically. Check /jobs and resend only the request you need."},
}

type Correlation struct {
	UpdateID  int64  `json:"update_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	JobID     string `json:"job_id,omitempty"`
}

func (c Correlation) valid() bool {
	return c.UpdateID >= 0 && (c.SessionID == "" || files.ValidID(c.SessionID)) && (c.JobID == "" || files.ValidID(c.JobID))
}

type Occurrence struct {
	At          time.Time   `json:"at"`
	Correlation Correlation `json:"correlation"`
}

type Report struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Code    string `json:"code"`
	Diagnostic
	Count        uint64       `json:"count"`
	Acknowledged uint64       `json:"acknowledged_count"`
	FirstAt      time.Time    `json:"first_at"`
	LastAt       time.Time    `json:"last_at"`
	Recent       []Occurrence `json:"recent"`
	Storage      string       `json:"storage,omitempty"`
}

func ID(code string) string { return files.Digest([]byte("chunsu-error:" + code)) }

func add(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

// Recorder retains a bounded pending aggregate when persistence is temporarily
// unavailable. A later Flush retries it; Unpersisted is exposed in chat health.
type Recorder struct {
	Root    string
	Limits  config.Limits
	mu      sync.Mutex
	pending map[string]Report
}

func New(root string, limits config.Limits) *Recorder {
	return &Recorder{Root: root, Limits: limits, pending: make(map[string]Report)}
}

func (r *Recorder) Record(ctx context.Context, code string, correlation Correlation) (string, error) {
	diagnostic, ok := catalog[code]
	if !ok || !correlation.valid() {
		return "", errors.New("invalid error report classification or correlation")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	p := r.pending[code]
	if p.Count == 0 {
		p = Report{Version: Version, ID: ID(code), Code: code, Diagnostic: diagnostic, FirstAt: now}
	}
	p.Count = add(p.Count, 1)
	p.LastAt = now
	p.Recent = recent(append(p.Recent, Occurrence{At: now, Correlation: correlation}), r.Limits.MaxMessages)
	r.pending[code] = p
	return p.ID, r.flush(ctx)
}

func recent(items []Occurrence, limit int) []Occurrence {
	if limit < 1 {
		limit = 1
	}
	if len(items) > limit {
		items = append([]Occurrence(nil), items[len(items)-limit:]...)
	}
	return items
}

func lock(ctx context.Context, root string, limits config.Limits) (*platform.Lock, error) {
	if err := files.RequirePrivateDir(root); err != nil {
		return nil, err
	}
	base := filepath.Join(root, Directory)
	for _, path := range []string{filepath.Join(root, "state"), base, filepath.Join(base, "state")} {
		if err := files.PrivateDir(path); err != nil {
			return nil, err
		}
	}
	return platform.Acquire(ctx, base, time.Duration(limits.LockWaitSeconds)*time.Second)
}

func (r *Recorder) Flush(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flush(ctx)
}

func (r *Recorder) Unpersisted() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n uint64
	for _, p := range r.pending {
		n = add(n, p.Count)
	}
	return n
}

func (r *Recorder) flush(ctx context.Context) error {
	if len(r.pending) == 0 {
		return nil
	}
	l, err := lock(ctx, r.Root, r.Limits)
	if err != nil {
		return err
	}
	defer l.Close()
	var failures []error
	for code, p := range r.pending {
		old, err := read(r.Root, code, r.Limits)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
			continue
		}
		if err == nil {
			p.FirstAt = old.FirstAt
			p.Count = add(old.Count, p.Count)
			p.Acknowledged = old.Acknowledged
			p.Recent = recent(append(old.Recent, p.Recent...), r.Limits.MaxMessages)
		}
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		if int64(len(data)) > r.Limits.MaxArtifactBytes {
			failures = append(failures, errors.New("error report exceeds configured artifact limit"))
			continue
		}
		if err = files.Write(r.Root, filepath.Join(Directory, code+".json"), append(data, '\n'), true); err != nil {
			failures = append(failures, err)
			continue
		}
		delete(r.pending, code)
	}
	return errors.Join(failures...)
}

func read(root, code string, limits config.Limits) (Report, error) {
	var p Report
	path := filepath.Join(Directory, code+".json")
	if err := files.RequirePrivateFile(filepath.Join(root, path)); err != nil {
		return p, err
	}
	data, err := files.Read(root, path, limits.MaxArtifactBytes)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return Report{}, errors.New("invalid error report")
	}
	if p.Version != Version || p.ID != ID(code) || p.Code != code || p.Acknowledged > p.Count || p.FirstAt.IsZero() || p.LastAt.Before(p.FirstAt) {
		return Report{}, errors.New("invalid error report metadata")
	}
	for _, occurrence := range p.Recent {
		if !occurrence.Correlation.valid() {
			return Report{}, errors.New("invalid error correlation")
		}
	}
	// Text is always regenerated from the trusted catalog, never echoed from disk.
	p.Diagnostic = catalog[code]
	p.Storage = "retained"
	p.Recent = recent(p.Recent, limits.MaxMessages)
	return p, nil
}

func List(root string, limits config.Limits) ([]Report, error) {
	reports := []Report{}
	for code := range catalog {
		p, err := read(root, code, limits)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			// Preserve a damaged group while keeping other reports inspectable.
			p = Report{Version: Version, ID: ID(code), Code: code, Storage: "unreadable", Diagnostic: Diagnostic{Component: "error_store", Summary: "Error report could not be read: " + code, Recovery: "Check the host error-report file, permissions, and available storage. The unreadable file was not overwritten."}}
		}
		reports = append(reports, p)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].LastAt.After(reports[j].LastAt) })
	return reports, nil
}

func Acknowledge(ctx context.Context, root, id string, limits config.Limits) error {
	l, err := lock(ctx, root, limits)
	if err != nil {
		return err
	}
	defer l.Close()
	for code := range catalog {
		if id != ID(code) {
			continue
		}
		p, err := read(root, code, limits)
		if err != nil {
			return err
		}
		p.Acknowledged = p.Count
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		return files.Write(root, filepath.Join(Directory, code+".json"), append(data, '\n'), true)
	}
	return fmt.Errorf("unknown error report ID")
}
