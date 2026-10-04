// Package ops owns the local business ledger. Scope and actor labels are
// organizational metadata; access is the existing OS-owner boundary.
package ops

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
)

const Version = 1
const Workgroup = "team-ops"
const RecordKind = "ops_event"

const (
	Ready      = "ready"
	InProgress = "inprogress"
	Waiting    = "waiting"
	Completed  = "completed"
	Cancelled  = "cancelled"
)

type Scope struct {
	Team    string `json:"team"`
	Project string `json:"project"`
}

func (s Scope) Validate() error {
	if !identifier(s.Team) || !identifier(s.Project) {
		return errors.New("OPS requires explicit team and project identifiers")
	}
	return nil
}

func (s Scope) Key() string {
	b, _ := json.Marshal(s)
	return "ops:" + files.Digest(b)
}

type RecordRef struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

type Command struct {
	Version          int             `json:"version"`
	Scope            Scope           `json:"scope"`
	OperationID      string          `json:"operation_id"`
	ExpectedRevision int64           `json:"expected_revision"`
	Actor            string          `json:"actor"`
	Reason           string          `json:"reason"`
	Action           string          `json:"action"`
	EntityID         string          `json:"entity_id,omitempty"`
	Data             json.RawMessage `json:"data"`
}

type Event struct {
	Version       int       `json:"version"`
	Scope         Scope     `json:"scope"`
	Revision      int64     `json:"revision"`
	Previous      RecordRef `json:"previous"`
	At            string    `json:"at"`
	CommandDigest string    `json:"command_digest"`
	Command       Command   `json:"command"`
}

type HistoryEntry struct {
	Record store.Record `json:"record"`
	Event  Event        `json:"event"`
}

type Receipt struct {
	Record      store.Record `json:"record"`
	Revision    int64        `json:"revision"`
	OperationID string       `json:"operation_id"`
	Replayed    bool         `json:"replayed"`
}

type Settings struct {
	Timezone  string `json:"timezone"`
	Synthetic bool   `json:"synthetic"`
}

type Evidence struct {
	ObservationID string   `json:"observation_id,omitempty"`
	SourceID      string   `json:"source_id,omitempty"`
	SpanIDs       []string `json:"span_ids,omitempty"`
	Statement     string   `json:"statement"`
	URI           string   `json:"uri,omitempty"`
}

type WaitDetail struct {
	Reason      string `json:"reason"`
	Responsible string `json:"responsible"`
	NextCheckAt string `json:"next_check_at"`
}

type ScheduleRevision struct {
	PreviousDue    string `json:"previous_due"`
	Due            string `json:"due"`
	Reason         string `json:"reason"`
	Actor          string `json:"actor"`
	At             string `json:"at"`
	LedgerRevision int64  `json:"ledger_revision"`
}

type WorkItem struct {
	ID                 string             `json:"id"`
	Title              string             `json:"title"`
	Owner              string             `json:"owner"`
	CompletionCriteria string             `json:"completion_criteria"`
	State              string             `json:"state"`
	OriginalDue        string             `json:"original_due"`
	CurrentDue         string             `json:"current_due"`
	ScheduleRevisions  []ScheduleRevision `json:"schedule_revisions"`
	Wait               *WaitDetail        `json:"wait,omitempty"`
	Evidence           []Evidence         `json:"evidence"`
	CompletedAt        string             `json:"completed_at,omitempty"`
	CreatedAt          string             `json:"created_at"`
	UpdatedAt          string             `json:"updated_at"`
}

type WorkCreate struct {
	Title              string     `json:"title"`
	Owner              string     `json:"owner"`
	CompletionCriteria string     `json:"completion_criteria"`
	Due                string     `json:"due"`
	Evidence           []Evidence `json:"evidence"`
}

type WorkTransition struct {
	State    string      `json:"state"`
	Wait     *WaitDetail `json:"wait,omitempty"`
	Evidence []Evidence  `json:"evidence"`
}

type Reschedule struct {
	Due string `json:"due"`
}

type Commitment struct {
	WorkID    string `json:"work_id"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Actor     string `json:"actor"`
	At        string `json:"at"`
	Revision  int64  `json:"revision"`
	CarriedTo string `json:"carried_to,omitempty"`
}

type Deployment struct {
	Status      string     `json:"status"`
	Environment string     `json:"environment"`
	WorkIDs     []string   `json:"work_ids"`
	EffectiveAt string     `json:"effective_at"`
	Evidence    []Evidence `json:"evidence"`
	Actor       string     `json:"actor"`
	RecordedAt  string     `json:"recorded_at"`
}

type ReleaseCycle struct {
	ID                 string       `json:"id"`
	Service            string       `json:"service"`
	Environment        string       `json:"environment"`
	ReadyBy            string       `json:"ready_by"`
	DeployBy           string       `json:"deploy_by"`
	CommitmentRevision int64        `json:"commitment_revision"`
	Commitments        []Commitment `json:"commitments"`
	Deployments        []Deployment `json:"deployments"`
}

type ReleaseCreate struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	ReadyBy     string `json:"ready_by"`
	DeployBy    string `json:"deploy_by"`
}

type TargetChange struct {
	WorkID    string `json:"work_id"`
	Status    string `json:"status"`
	CarriedTo string `json:"carried_to,omitempty"`
}

type Span struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Source struct {
	ID            string `json:"id"`
	LogicalID     string `json:"logical_id"`
	Kind          string `json:"kind"`
	ChangeKind    string `json:"change_kind"`
	ExternalID    string `json:"external_id"`
	EventID       string `json:"event_id"`
	Version       string `json:"version"`
	ThreadID      string `json:"thread_id"`
	ChannelID     string `json:"channel_id"`
	URI           string `json:"uri"`
	SourceAt      string `json:"source_at"`
	CapturedAt    string `json:"captured_at"`
	ContentStatus string `json:"content_status"`
	Spans         []Span `json:"spans"`
}

type Observation struct {
	ID              string   `json:"id"`
	Version         int      `json:"version"`
	Scope           Scope    `json:"scope"`
	Provider        string   `json:"provider"`
	Synthetic       bool     `json:"synthetic"`
	CapturedFrom    string   `json:"captured_from"`
	CapturedThrough string   `json:"captured_through"`
	Coverage        string   `json:"coverage"`
	Gaps            []string `json:"gaps"`
	Sources         []Source `json:"sources"`
	InputDigest     string   `json:"input_digest"`
	Raw             []byte   `json:"raw"`
}

type SeverityRule struct {
	Mode                  string   `json:"mode"`
	Recipients            []string `json:"recipients"`
	CooldownSeconds       int64    `json:"cooldown_seconds"`
	AcknowledgmentSeconds int64    `json:"acknowledgment_seconds"`
}

type Policy struct {
	Version                int                     `json:"version"`
	Rules                  map[string]SeverityRule `json:"rules"`
	SourceFreshnessSeconds int64                   `json:"source_freshness_seconds"`
	P0BypassesSnooze       bool                    `json:"p0_bypasses_snooze"`
}

func DefaultPolicy() Policy {
	return Policy{Version: Version, Rules: map[string]SeverityRule{
		"P0": {Mode: "immediate", Recipients: []string{}},
		"P1": {Mode: "immediate", Recipients: []string{}},
		"P2": {Mode: "digest", Recipients: []string{}},
		"P3": {Mode: "none", Recipients: []string{}},
	}}
}

type Citation struct {
	SourceID string   `json:"source_id"`
	SpanIDs  []string `json:"span_ids"`
}

type Risk struct {
	Key               string     `json:"key"`
	Kind              string     `json:"kind"`
	WorkID            string     `json:"work_id"`
	ReleaseID         string     `json:"release_id"`
	IncidentID        string     `json:"incident_id"`
	Severity          string     `json:"severity"`
	Confidence        string     `json:"confidence"`
	Title             string     `json:"title"`
	Impact            string     `json:"impact"`
	Rationale         string     `json:"rationale"`
	RecommendedAction string     `json:"recommended_action"`
	Owner             string     `json:"owner"`
	Citations         []Citation `json:"citations"`
	Uncertainties     []string   `json:"uncertainties"`
}

type Incident struct {
	ID                 string     `json:"id"`
	Key                string     `json:"key"`
	Revision           int64      `json:"revision"`
	State              string     `json:"state"`
	Risk               Risk       `json:"risk"`
	Fingerprint        string     `json:"fingerprint"`
	AssessmentJobID    string     `json:"assessment_job_id"`
	AssessmentArtifact RecordRef  `json:"assessment_artifact"`
	AcknowledgedBy     string     `json:"acknowledged_by,omitempty"`
	AcknowledgedAt     string     `json:"acknowledged_at,omitempty"`
	SnoozedUntil       string     `json:"snoozed_until,omitempty"`
	ResolutionEvidence []Evidence `json:"resolution_evidence"`
	CreatedAt          string     `json:"created_at"`
	UpdatedAt          string     `json:"updated_at"`
}

type Notification struct {
	ID                string   `json:"id"`
	IncidentID        string   `json:"incident_id"`
	IncidentRevision  int64    `json:"incident_revision"`
	Key               string   `json:"key"`
	Severity          string   `json:"severity"`
	Mode              string   `json:"mode"`
	Recipients        []string `json:"recipients"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason"`
	CreatedAt         string   `json:"created_at"`
	AcknowledgmentDue string   `json:"acknowledgment_due,omitempty"`
	Policy            Policy   `json:"policy"`
}

type IncidentChange struct {
	State        string     `json:"state"`
	SnoozedUntil string     `json:"snoozed_until,omitempty"`
	Evidence     []Evidence `json:"evidence"`
}

type Assessment struct {
	JobID          string    `json:"job_id"`
	Artifact       RecordRef `json:"artifact"`
	InputDigest    string    `json:"input_digest"`
	LedgerRevision int64     `json:"ledger_revision"`
	BundleDigest   string    `json:"bundle_digest"`
	Report         Report    `json:"report"`
}

type State struct {
	Version       int                     `json:"version"`
	Scope         Scope                   `json:"scope"`
	Revision      int64                   `json:"revision"`
	Head          RecordRef               `json:"head"`
	Settings      Settings                `json:"settings"`
	Policy        Policy                  `json:"policy"`
	WorkItems     map[string]WorkItem     `json:"work_items"`
	ReleaseCycles map[string]ReleaseCycle `json:"release_cycles"`
	Observations  map[string]Observation  `json:"observations"`
	Incidents     map[string]Incident     `json:"incidents"`
	Notifications []Notification          `json:"notifications"`
	Assessments   []Assessment            `json:"assessments"`
}

func emptyState(scope Scope) State {
	return State{Version: Version, Scope: scope, Policy: DefaultPolicy(), WorkItems: map[string]WorkItem{}, ReleaseCycles: map[string]ReleaseCycle{}, Observations: map[string]Observation{}, Incidents: map[string]Incident{}, Notifications: []Notification{}, Assessments: []Assessment{}}
}

func identifier(value string) bool {
	if len(value) == 0 || len(value) > files.MaxIDLength {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return value != "." && value != ".."
}

func timestamp(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func decode(data []byte, out any) error { return mail.Decode(data, out) }

func nonempty(value string) bool { return strings.TrimSpace(value) != "" }

func validDuration(seconds int64) bool { return seconds >= 0 && seconds <= config.MaxDurationSeconds }
