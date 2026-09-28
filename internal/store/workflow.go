package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"chunsu/internal/files"
)

const (
	Staged           = "staged"
	StepPending      = "pending"
	StepReady        = "ready"
	StepRunning      = "running"
	StepWaitingInput = "waiting_input"
	StepWaitingAuth  = "waiting_auth"
	StepRetryWait    = "retry_wait"
	StepCompleted    = "completed"
	StepFailed       = "failed"
	StepCancelled    = "cancelled"
	StepSuperseded   = "superseded"
	StepInterrupted  = "interrupted"
	MaxWorkflowSteps = 32
)

var ErrStaleStep = errors.New("stale step attempt or state")
var ErrStepBlocked = errors.New("step dependencies, pause, or gate block execution")

type PinnedArtifactRef struct {
	ArtifactID string `json:"artifact_id"`
	Digest     string `json:"digest"`
}

type StepSpec struct {
	Key                  string              `json:"key"`
	Stage                string              `json:"stage"`
	DependsOn            []string            `json:"depends_on,omitempty"`
	InputArtifacts       []PinnedArtifactRef `json:"input_artifacts,omitempty"`
	ExpectedOutputSchema string              `json:"expected_output_schema"`
	ExecutorModel        string              `json:"executor_model"`
	ExecutorEffort       string              `json:"executor_effort"`
	ExecutorIdentity     string              `json:"executor_identity"`
	InstructionVersion   string              `json:"instruction_version"`
	GatePolicy           string              `json:"gate_policy,omitempty"`
}

type WorkflowSpec struct {
	OriginConversationID string          `json:"origin_conversation_id"`
	OriginMessageID      string          `json:"origin_message_id"`
	RequestRevision      int64           `json:"request_revision"`
	OriginalRequest      json.RawMessage `json:"original_request"`
	CompletionCriteria   json.RawMessage `json:"completion_criteria"`
	SourceScope          json.RawMessage `json:"source_scope"`
	Budget               json.RawMessage `json:"budget"`
	Destination          json.RawMessage `json:"destination"`
	WorkflowVersion      string          `json:"workflow_version"`
	ManualCheckpoint     bool            `json:"manual_checkpoint"`
	Steps                []StepSpec      `json:"steps"`
}

type WorkflowJob struct {
	JobID                string          `json:"job_id"`
	OriginConversationID string          `json:"origin_conversation_id"`
	OriginMessageID      string          `json:"origin_message_id"`
	RequestRevision      int64           `json:"request_revision"`
	OriginalRequest      json.RawMessage `json:"original_request"`
	CompletionCriteria   json.RawMessage `json:"completion_criteria"`
	SourceScope          json.RawMessage `json:"source_scope"`
	Budget               json.RawMessage `json:"budget"`
	Destination          json.RawMessage `json:"destination"`
	WorkflowVersion      string          `json:"workflow_version"`
	Paused               bool            `json:"paused"`
	ManualCheckpoint     bool            `json:"manual_checkpoint"`
	CreatedAt            int64           `json:"created_at"`
	UpdatedAt            int64           `json:"updated_at"`
}

type Step struct {
	ID                   string              `json:"id"`
	JobID                string              `json:"job_id"`
	Key                  string              `json:"key"`
	Stage                string              `json:"stage"`
	Ordinal              int                 `json:"ordinal"`
	DependsOn            []string            `json:"depends_on"`
	InputArtifacts       []PinnedArtifactRef `json:"input_artifacts"`
	ExpectedOutputSchema string              `json:"expected_output_schema"`
	ExecutorModel        string              `json:"executor_model"`
	ExecutorEffort       string              `json:"executor_effort"`
	ExecutorIdentity     string              `json:"executor_identity"`
	InstructionVersion   string              `json:"instruction_version"`
	GatePolicy           string              `json:"gate_policy"`
	State                string              `json:"state"`
	CurrentAttemptID     string              `json:"current_attempt_id,omitempty"`
	OutputArtifactID     string              `json:"output_artifact_id,omitempty"`
	NotBefore            int64               `json:"not_before,omitempty"`
	Diagnostic           string              `json:"diagnostic,omitempty"`
	CreatedAt            int64               `json:"created_at"`
	UpdatedAt            int64               `json:"updated_at"`
}

type StepAttempt struct {
	ID                 string              `json:"id"`
	StepID             string              `json:"step_id"`
	Ordinal            int                 `json:"ordinal"`
	Status             string              `json:"status"`
	StartedAt          int64               `json:"started_at"`
	EndedAt            int64               `json:"ended_at,omitempty"`
	Inputs             []PinnedArtifactRef `json:"inputs"`
	OutputArtifactID   string              `json:"output_artifact_id,omitempty"`
	ExecutorModel      string              `json:"executor_model"`
	ExecutorEffort     string              `json:"executor_effort"`
	ExecutorIdentity   string              `json:"executor_identity"`
	InstructionVersion string              `json:"instruction_version"`
	Diagnostic         string              `json:"diagnostic,omitempty"`
}

const workflowStepColumns = "id,job_id,step_key,stage,ordinal,dependencies_json,pinned_inputs_json,expected_output_schema,executor_model,executor_effort,executor_identity,instruction_version,gate_policy,state,current_attempt_id,output_artifact_id,not_before,diagnostic,created_at,updated_at"

func scanStep(row scanner) (Step, error) {
	var s Step
	var dependencies, inputs string
	err := row.Scan(&s.ID, &s.JobID, &s.Key, &s.Stage, &s.Ordinal, &dependencies, &inputs, &s.ExpectedOutputSchema, &s.ExecutorModel, &s.ExecutorEffort, &s.ExecutorIdentity, &s.InstructionVersion, &s.GatePolicy, &s.State, &s.CurrentAttemptID, &s.OutputArtifactID, &s.NotBefore, &s.Diagnostic, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal([]byte(dependencies), &s.DependsOn); err != nil {
		return s, err
	}
	if err = json.Unmarshal([]byte(inputs), &s.InputArtifacts); err != nil {
		return s, err
	}
	return s, nil
}

func scanStepAttempt(row scanner) (StepAttempt, error) {
	var a StepAttempt
	var inputs string
	err := row.Scan(&a.ID, &a.StepID, &a.Ordinal, &a.Status, &a.StartedAt, &a.EndedAt, &inputs, &a.OutputArtifactID, &a.ExecutorModel, &a.ExecutorEffort, &a.ExecutorIdentity, &a.InstructionVersion, &a.Diagnostic)
	if err != nil {
		return a, err
	}
	err = json.Unmarshal([]byte(inputs), &a.Inputs)
	return a, err
}

func jsonValue(v json.RawMessage) string {
	if len(v) == 0 {
		return "null"
	}
	return string(v)
}

func validateSteps(steps []StepSpec, prior map[string]bool, total int) error {
	if len(steps) == 0 || total > MaxWorkflowSteps {
		return fmt.Errorf("workflow requires 1-%d steps", MaxWorkflowSteps)
	}
	for _, step := range steps {
		if !validStepKey(step.Key) || step.Stage == "" || step.ExpectedOutputSchema == "" || step.ExecutorIdentity == "" || step.InstructionVersion == "" {
			return errors.New("step key, stage, output schema, executor identity and instruction version are required")
		}
		if prior[step.Key] {
			return fmt.Errorf("duplicate step key %q", step.Key)
		}
		for _, dep := range step.DependsOn {
			if !prior[dep] {
				return fmt.Errorf("step %q dependency %q must be an earlier step", step.Key, dep)
			}
		}
		if step.GatePolicy != "" && step.GatePolicy != "auto" && step.GatePolicy != "manual" {
			return errors.New("unsupported step gate policy")
		}
		prior[step.Key] = true
	}
	return nil
}

func (s *Store) SubmitStaged(ctx context.Context, group string, input []byte, request any, spec WorkflowSpec, limit int64) (Job, error) {
	var j Job
	if group == "" || spec.OriginConversationID == "" || spec.WorkflowVersion == "" || spec.RequestRevision < 0 {
		return j, errors.New("workgroup, origin conversation, workflow version and nonnegative request revision are required")
	}
	if limit <= 0 || int64(len(input)) > limit {
		return j, errors.New("input exceeds configured limit")
	}
	if err := validateSteps(spec.Steps, map[string]bool{}, len(spec.Steps)); err != nil {
		return j, err
	}
	for _, raw := range []json.RawMessage{spec.OriginalRequest, spec.CompletionCriteria, spec.SourceScope, spec.Budget, spec.Destination} {
		if len(raw) > 0 && !json.Valid(raw) {
			return j, errors.New("invalid workflow metadata JSON")
		}
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return j, err
	}
	j = Job{ID: files.ID(), Workgroup: group, Status: Staged, CreatedAt: now(), UpdatedAt: now(), Request: requestJSON}
	j.InputRef = filepath.ToSlash(filepath.Join("runs", j.ID, "input.json"))
	if err = files.Write(s.Root, j.InputRef, input, false); err != nil {
		return j, err
	}
	inputArtifact := Artifact{ID: files.ID(), JobID: j.ID, Kind: "input", Path: j.InputRef, Digest: files.Digest(input), Bytes: int64(len(input)), CreatedAt: now(), ContentState: ContentAvailable}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return j, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO jobs(id,workgroup,input_ref,status,created_at,updated_at,request_json) VALUES(?,?,?,?,?,?,?)", j.ID, j.Workgroup, j.InputRef, j.Status, j.CreatedAt, j.UpdatedAt, string(requestJSON)); err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO artifacts(id,job_id,kind,path,digest,bytes,created_at) VALUES(?,?,?,?,?,?,?)", inputArtifact.ID, j.ID, inputArtifact.Kind, inputArtifact.Path, inputArtifact.Digest, inputArtifact.Bytes, inputArtifact.CreatedAt); err != nil {
		return j, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO workflow_jobs(job_id,origin_conversation_id,origin_message_id,request_revision,original_request_json,completion_criteria_json,source_scope_json,budget_json,destination_json,workflow_version,manual_checkpoint,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", j.ID, spec.OriginConversationID, spec.OriginMessageID, spec.RequestRevision, jsonValue(spec.OriginalRequest), jsonValue(spec.CompletionCriteria), jsonValue(spec.SourceScope), jsonValue(spec.Budget), jsonValue(spec.Destination), spec.WorkflowVersion, spec.ManualCheckpoint, j.CreatedAt, j.UpdatedAt); err != nil {
		return j, err
	}
	for ordinal, step := range spec.Steps {
		inputs := append([]PinnedArtifactRef(nil), step.InputArtifacts...)
		if ordinal == 0 && len(inputs) == 0 {
			inputs = []PinnedArtifactRef{{ArtifactID: inputArtifact.ID, Digest: inputArtifact.Digest}}
		}
		if err = insertStep(ctx, tx, j.ID, ordinal+1, step, inputs); err != nil {
			return j, err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,kind,created_at,data_json) VALUES(?,?,?,?)", j.ID, "workflow.submitted", now(), `{}`); err != nil {
		return j, err
	}
	return j, tx.Commit()
}

func insertStep(ctx context.Context, tx *sql.Tx, jobID string, ordinal int, spec StepSpec, inputs []PinnedArtifactRef) error {
	dependencies, err := json.Marshal(spec.DependsOn)
	if err != nil {
		return err
	}
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return err
	}
	gate := spec.GatePolicy
	if gate == "" {
		gate = "auto"
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO workflow_steps(id,job_id,step_key,stage,ordinal,dependencies_json,pinned_inputs_json,expected_output_schema,executor_model,executor_effort,executor_identity,instruction_version,gate_policy,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", files.ID(), jobID, spec.Key, spec.Stage, ordinal, string(dependencies), string(inputJSON), spec.ExpectedOutputSchema, spec.ExecutorModel, spec.ExecutorEffort, spec.ExecutorIdentity, spec.InstructionVersion, gate, StepPending, now(), now())
	return err
}

func (s *Store) Workflow(ctx context.Context, jobID string) (WorkflowJob, error) {
	var w WorkflowJob
	var original, criteria, scope, budget, destination string
	err := s.DB.QueryRowContext(ctx, "SELECT job_id,origin_conversation_id,origin_message_id,request_revision,original_request_json,completion_criteria_json,source_scope_json,budget_json,destination_json,workflow_version,paused,manual_checkpoint,created_at,updated_at FROM workflow_jobs WHERE job_id=?", jobID).Scan(&w.JobID, &w.OriginConversationID, &w.OriginMessageID, &w.RequestRevision, &original, &criteria, &scope, &budget, &destination, &w.WorkflowVersion, &w.Paused, &w.ManualCheckpoint, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return w, err
	}
	w.OriginalRequest = json.RawMessage(original)
	w.CompletionCriteria = json.RawMessage(criteria)
	w.SourceScope = json.RawMessage(scope)
	w.Budget = json.RawMessage(budget)
	w.Destination = json.RawMessage(destination)
	return w, nil
}

func (s *Store) Steps(ctx context.Context, jobID string) ([]Step, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE job_id=? ORDER BY ordinal", jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		v, e := scanStep(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Step(ctx context.Context, stepID string) (Step, error) {
	return scanStep(s.DB.QueryRowContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE id=?", stepID))
}

func (s *Store) StepAttempts(ctx context.Context, stepID string) ([]StepAttempt, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT id,step_id,ordinal,status,started_at,ended_at,inputs_json,output_artifact_id,executor_model,executor_effort,executor_identity,instruction_version,diagnostic FROM step_attempts WHERE step_id=? ORDER BY ordinal", stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StepAttempt{}
	for rows.Next() {
		v, e := scanStepAttempt(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func stepListTx(ctx context.Context, tx *sql.Tx, jobID string) ([]Step, error) {
	rows, err := tx.QueryContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE job_id=? ORDER BY ordinal", jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		v, e := scanStep(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func resolvedInputs(steps []Step, step Step) ([]PinnedArtifactRef, bool) {
	byKey := map[string]Step{}
	for _, v := range steps {
		byKey[v.Key] = v
	}
	inputs := append([]PinnedArtifactRef(nil), step.InputArtifacts...)
	for _, key := range step.DependsOn {
		dep, ok := byKey[key]
		if !ok || dep.State != StepCompleted || dep.OutputArtifactID == "" {
			return nil, false
		}
		// The digest is loaded inside the same transaction by pinInputsTx.
		inputs = append(inputs, PinnedArtifactRef{ArtifactID: dep.OutputArtifactID})
	}
	return inputs, true
}

func pinInputsTx(ctx context.Context, tx *sql.Tx, jobID string, refs []PinnedArtifactRef) ([]PinnedArtifactRef, error) {
	seen := map[string]bool{}
	out := make([]PinnedArtifactRef, 0, len(refs))
	for _, ref := range refs {
		if seen[ref.ArtifactID] {
			continue
		}
		var digest, state string
		err := tx.QueryRowContext(ctx, "SELECT digest,content_state FROM artifacts WHERE id=? AND job_id=?", ref.ArtifactID, jobID).Scan(&digest, &state)
		if err != nil {
			return nil, err
		}
		if state != ContentAvailable || ref.Digest != "" && ref.Digest != digest {
			return nil, errors.New("pinned artifact is unavailable or digest changed")
		}
		seen[ref.ArtifactID] = true
		out = append(out, PinnedArtifactRef{ArtifactID: ref.ArtifactID, Digest: digest})
	}
	return out, nil
}

// ReadyStep promotes only the earliest eligible step. It does not cross a manual
// checkpoint and never exposes work from a paused or cancelled job.
func (s *Store) ReadyStep(ctx context.Context, jobID string) (Step, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Step{}, err
	}
	defer tx.Rollback()
	var paused, manual bool
	var status string
	err = tx.QueryRowContext(ctx, "SELECT w.paused,w.manual_checkpoint,j.status FROM workflow_jobs w JOIN jobs j ON j.id=w.job_id WHERE w.job_id=?", jobID).Scan(&paused, &manual, &status)
	if err != nil {
		return Step{}, err
	}
	if paused || status != Staged {
		return Step{}, sql.ErrNoRows
	}
	steps, err := stepListTx(ctx, tx, jobID)
	if err != nil {
		return Step{}, err
	}
	for _, step := range steps {
		if step.State == StepReady {
			return step, tx.Commit()
		}
		if step.State != StepPending && step.State != StepRetryWait {
			continue
		}
		if step.NotBefore > now() {
			continue
		}
		refs, ok := resolvedInputs(steps, step)
		if !ok {
			continue
		}
		refs, err = pinInputsTx(ctx, tx, jobID, refs)
		if err != nil {
			return Step{}, err
		}
		inputJSON, err := json.Marshal(refs)
		if err != nil {
			return Step{}, err
		}
		state := StepReady
		if step.GatePolicy == "manual" || manual && step.Ordinal > 1 {
			state = StepWaitingInput
		}
		result, err := tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,pinned_inputs_json=?,updated_at=? WHERE id=? AND state=?", state, string(inputJSON), now(), step.ID, step.State)
		if err != nil {
			return Step{}, err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return Step{}, ErrStaleStep
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,kind,created_at,data_json) VALUES(?,?,?,?)", jobID, "step."+state, now(), `{}`); err != nil {
			return Step{}, err
		}
		if err = tx.Commit(); err != nil {
			return Step{}, err
		}
		step.State = state
		step.InputArtifacts = refs
		if state == StepReady {
			return step, nil
		}
		return Step{}, sql.ErrNoRows
	}
	return Step{}, sql.ErrNoRows
}

func (s *Store) ApproveStep(ctx context.Context, stepID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	step, err := scanStep(tx.QueryRowContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE id=?", stepID))
	if err != nil {
		return err
	}
	var paused bool
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT w.paused,j.status FROM workflow_jobs w JOIN jobs j ON j.id=w.job_id WHERE w.job_id=?", step.JobID).Scan(&paused, &status); err != nil {
		return err
	}
	if paused || status != Staged {
		return ErrStepBlocked
	}
	steps, err := stepListTx(ctx, tx, step.JobID)
	if err != nil {
		return err
	}
	if _, ok := resolvedInputs(steps, step); !ok {
		return ErrStepBlocked
	}
	result, err := tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,updated_at=? WHERE id=? AND state=?", StepReady, now(), stepID, StepWaitingInput)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrStaleStep
	}
	return tx.Commit()
}

func (s *Store) ClaimStep(ctx context.Context, stepID, expectedAttemptID string) (StepAttempt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return StepAttempt{}, err
	}
	defer tx.Rollback()
	step, err := scanStep(tx.QueryRowContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE id=?", stepID))
	if err != nil {
		return StepAttempt{}, err
	}
	var paused bool
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT w.paused,j.status FROM workflow_jobs w JOIN jobs j ON j.id=w.job_id WHERE w.job_id=?", step.JobID).Scan(&paused, &status); err != nil {
		return StepAttempt{}, err
	}
	if paused || status != Staged {
		return StepAttempt{}, ErrStepBlocked
	}
	if step.State != StepReady || step.CurrentAttemptID != expectedAttemptID {
		return StepAttempt{}, ErrStaleStep
	}
	steps, err := stepListTx(ctx, tx, step.JobID)
	if err != nil {
		return StepAttempt{}, err
	}
	refs, ok := resolvedInputs(steps, step)
	if !ok {
		return StepAttempt{}, ErrStepBlocked
	}
	refs, err = pinInputsTx(ctx, tx, step.JobID, refs)
	if err != nil {
		return StepAttempt{}, err
	}
	inputJSON, err := json.Marshal(refs)
	if err != nil {
		return StepAttempt{}, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM step_attempts WHERE step_id=?", stepID).Scan(&count); err != nil {
		return StepAttempt{}, err
	}
	a := StepAttempt{ID: files.ID(), StepID: stepID, Ordinal: count + 1, Status: StepRunning, StartedAt: now(), Inputs: refs, ExecutorModel: step.ExecutorModel, ExecutorEffort: step.ExecutorEffort, ExecutorIdentity: step.ExecutorIdentity, InstructionVersion: step.InstructionVersion}
	if _, err = tx.ExecContext(ctx, "INSERT INTO step_attempts(id,step_id,ordinal,status,started_at,inputs_json,executor_model,executor_effort,executor_identity,instruction_version) VALUES(?,?,?,?,?,?,?,?,?,?)", a.ID, a.StepID, a.Ordinal, a.Status, a.StartedAt, string(inputJSON), a.ExecutorModel, a.ExecutorEffort, a.ExecutorIdentity, a.InstructionVersion); err != nil {
		return StepAttempt{}, err
	}
	result, err := tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,current_attempt_id=?,pinned_inputs_json=?,updated_at=? WHERE id=? AND state=? AND current_attempt_id=?", StepRunning, a.ID, string(inputJSON), now(), stepID, StepReady, expectedAttemptID)
	if err != nil {
		return StepAttempt{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return StepAttempt{}, ErrStaleStep
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", step.JobID, a.ID, "step.started", now(), `{}`); err != nil {
		return StepAttempt{}, err
	}
	return a, tx.Commit()
}

// SaveStepArtifact registers an output only while its step attempt is still
// current and running. A late child may leave an unreferenced private file,
// but cannot publish an artifact row or a result after cancellation/retry.
func (s *Store) SaveStepArtifact(ctx context.Context, stepID, attemptID, kind string, data []byte, limit int64) (Artifact, error) {
	var a Artifact
	if !files.ValidID(stepID) || !files.ValidID(attemptID) || kind == "" {
		return a, errors.New("invalid step artifact identity")
	}
	if limit <= 0 || int64(len(data)) > limit {
		return a, errors.New("artifact exceeds configured limit")
	}
	var jobID string
	err := s.DB.QueryRowContext(ctx, "SELECT s.job_id FROM workflow_steps s JOIN jobs j ON j.id=s.job_id JOIN step_attempts a ON a.id=s.current_attempt_id WHERE s.id=? AND s.current_attempt_id=? AND s.state=? AND a.status=? AND j.status=?", stepID, attemptID, StepRunning, StepRunning, Staged).Scan(&jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, ErrStaleStep
		}
		return a, err
	}
	a = Artifact{ID: files.ID(), JobID: jobID, AttemptID: attemptID, Kind: kind, Digest: files.Digest(data), Bytes: int64(len(data)), CreatedAt: now(), ContentState: ContentAvailable}
	a.Path = filepath.ToSlash(filepath.Join("runs", jobID, "artifacts", a.ID))
	if strings.HasSuffix(kind, "_markdown") {
		a.Path += ".md"
	}
	if err = files.Write(s.Root, a.Path, data, false); err != nil {
		return a, err
	}
	result, err := s.DB.ExecContext(ctx, "INSERT INTO artifacts(id,job_id,attempt_id,kind,path,digest,bytes,created_at) SELECT ?,?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM workflow_steps s JOIN jobs j ON j.id=s.job_id JOIN step_attempts a ON a.id=s.current_attempt_id WHERE s.id=? AND s.job_id=? AND s.current_attempt_id=? AND s.state=? AND a.status=? AND j.status=?)", a.ID, a.JobID, a.AttemptID, a.Kind, a.Path, a.Digest, a.Bytes, a.CreatedAt, stepID, jobID, attemptID, StepRunning, StepRunning, Staged)
	if err != nil {
		return a, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return a, err
	}
	if n != 1 {
		return a, ErrStaleStep
	}
	return a, nil
}

func (s *Store) CompleteStep(ctx context.Context, stepID, attemptID, outputArtifactID, status string) (Step, error) {
	switch status {
	case StepCompleted, StepFailed, StepWaitingInput, StepWaitingAuth, StepRetryWait, StepCancelled:
	default:
		return Step{}, errors.New("invalid step completion status")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Step{}, err
	}
	defer tx.Rollback()
	step, err := scanStep(tx.QueryRowContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE id=?", stepID))
	if err != nil {
		return Step{}, err
	}
	var jobStatus string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id=?", step.JobID).Scan(&jobStatus); err != nil {
		return Step{}, err
	}
	if jobStatus != Staged || step.State != StepRunning || step.CurrentAttemptID != attemptID {
		return Step{}, ErrStaleStep
	}
	if status == StepCompleted && outputArtifactID == "" {
		return Step{}, errors.New("validated completion requires an artifact")
	}
	if outputArtifactID != "" {
		var artifact Artifact
		err = tx.QueryRowContext(ctx, "SELECT id,job_id,attempt_id,kind,path,digest,bytes,created_at,content_state FROM artifacts WHERE id=? AND job_id=?", outputArtifactID, step.JobID).Scan(&artifact.ID, &artifact.JobID, &artifact.AttemptID, &artifact.Kind, &artifact.Path, &artifact.Digest, &artifact.Bytes, &artifact.CreatedAt, &artifact.ContentState)
		if err != nil {
			return Step{}, err
		}
		if artifact.AttemptID != attemptID {
			return Step{}, errors.New("artifact belongs to another attempt")
		}
		if _, err = s.ReadArtifact(artifact, artifact.Bytes); err != nil {
			return Step{}, err
		}
	}
	stepOutput := ""
	if status == StepCompleted {
		stepOutput = outputArtifactID
	}
	result, err := tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,output_artifact_id=?,updated_at=? WHERE id=? AND state=? AND current_attempt_id=?", status, stepOutput, now(), stepID, StepRunning, attemptID)
	if err != nil {
		return Step{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Step{}, ErrStaleStep
	}
	result, err = tx.ExecContext(ctx, "UPDATE step_attempts SET status=?,ended_at=?,output_artifact_id=? WHERE id=? AND step_id=? AND status=?", status, now(), outputArtifactID, attemptID, stepID, StepRunning)
	if err != nil {
		return Step{}, err
	}
	n, _ = result.RowsAffected()
	if n != 1 {
		return Step{}, ErrStaleStep
	}
	data, _ := json.Marshal(map[string]string{"status": status, "artifact_id": outputArtifactID})
	if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", step.JobID, attemptID, "step.finished", now(), string(data)); err != nil {
		return Step{}, err
	}
	if err = tx.Commit(); err != nil {
		return Step{}, err
	}
	step.State = status
	step.OutputArtifactID = stepOutput
	step.UpdatedAt = now()
	return step, nil
}

func (s *Store) RetryStep(ctx context.Context, stepID, expectedAttemptID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	step, err := scanStep(tx.QueryRowContext(ctx, "SELECT "+workflowStepColumns+" FROM workflow_steps WHERE id=?", stepID))
	if err != nil {
		return err
	}
	if step.CurrentAttemptID != expectedAttemptID || expectedAttemptID == "" {
		return ErrStaleStep
	}
	if step.State != StepFailed && step.State != StepWaitingAuth && step.State != StepWaitingInput && step.State != StepRetryWait {
		return ErrStaleStep
	}
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id=?", step.JobID).Scan(&status); err != nil {
		return err
	}
	if status != Staged {
		return ErrStepBlocked
	}
	result, err := tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,not_before=0,updated_at=? WHERE id=? AND current_attempt_id=? AND state=?", StepRetryWait, now(), stepID, expectedAttemptID, step.State)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrStaleStep
	}
	return tx.Commit()
}

func (s *Store) PauseJob(ctx context.Context, jobID string) error {
	return s.setPaused(ctx, jobID, true)
}
func (s *Store) ResumeJob(ctx context.Context, jobID string) error {
	return s.setPaused(ctx, jobID, false)
}
func (s *Store) setPaused(ctx context.Context, jobID string, paused bool) error {
	result, err := s.DB.ExecContext(ctx, "UPDATE workflow_jobs SET paused=?,updated_at=? WHERE job_id=? AND EXISTS(SELECT 1 FROM jobs WHERE id=? AND status=?)", paused, now(), jobID, jobID, Staged)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrStepBlocked
	}
	return nil
}

// SupersedeJobRevision requires the controller to reconcile active processes
// before replacing a request. Old artifacts and attempts remain inspectable.
func (s *Store) SupersedeJobRevision(ctx context.Context, jobID string, expectedRevision, nextRevision int64) error {
	if nextRevision <= expectedRevision {
		return errors.New("revision must increase")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var running int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM workflow_steps WHERE job_id=? AND state=?", jobID, StepRunning).Scan(&running); err != nil {
		return err
	}
	if running > 0 {
		return ErrStepBlocked
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM result_events WHERE job_id=? AND delivery_status=?", jobID, ResultSending).Scan(&running); err != nil {
		return err
	}
	if running > 0 {
		return ErrStepBlocked
	}
	result, err := tx.ExecContext(ctx, "UPDATE workflow_jobs SET request_revision=?,updated_at=? WHERE job_id=? AND request_revision=? AND EXISTS(SELECT 1 FROM jobs WHERE id=? AND status=?)", nextRevision, now(), jobID, expectedRevision, jobID, Staged)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrStaleStep
	}
	if _, err = tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,updated_at=? WHERE job_id=? AND state NOT IN (?,?)", StepSuperseded, now(), jobID, StepCancelled, StepSuperseded); err != nil {
		return err
	}
	return tx.Commit()
}

// AppendSteps supports one bounded additional collection/refinement sequence.
// Step keys and dependencies remain ordered within the same durable job.
func (s *Store) AppendSteps(ctx context.Context, jobID string, expectedRevision int64, steps []StepSpec) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var revision int64
	var status string
	if err = tx.QueryRowContext(ctx, "SELECT w.request_revision,j.status FROM workflow_jobs w JOIN jobs j ON j.id=w.job_id WHERE w.job_id=?", jobID).Scan(&revision, &status); err != nil {
		return err
	}
	if revision != expectedRevision || status != Staged {
		return ErrStaleStep
	}
	current, err := stepListTx(ctx, tx, jobID)
	if err != nil {
		return err
	}
	for _, v := range current {
		if v.State == StepRunning {
			return ErrStepBlocked
		}
	}
	prior := map[string]bool{}
	for _, v := range current {
		prior[v.Key] = true
	}
	if err = validateSteps(steps, prior, len(current)+len(steps)); err != nil {
		return err
	}
	for i, v := range steps {
		if err = insertStep(ctx, tx, jobID, len(current)+i+1, v, nil); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecoverInterruptedSteps must run after the controller has exclusive ownership
// and has reconciled surviving child process identities. It never auto-retries.
func (s *Store) RecoverInterruptedSteps(ctx context.Context) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,job_id,current_attempt_id FROM workflow_steps WHERE state=?", StepRunning)
	if err != nil {
		return 0, err
	}
	type item struct{ id, job, attempt string }
	var items []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.job, &v.attempt); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, v := range items {
		if _, err = tx.ExecContext(ctx, "UPDATE step_attempts SET status=?,ended_at=?,diagnostic=? WHERE id=? AND status=?", StepInterrupted, now(), RecoveryRequired, v.attempt, StepRunning); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,diagnostic=?,updated_at=? WHERE id=? AND state=?", StepWaitingInput, RecoveryRequired, now(), v.id, StepRunning); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO events(job_id,attempt_id,kind,created_at,data_json) VALUES(?,?,?,?,?)", v.job, v.attempt, "step.interrupted", now(), `{}`); err != nil {
			return 0, err
		}
	}
	return len(items), tx.Commit()
}

func (s *Store) cancelStagedTx(ctx context.Context, tx *sql.Tx, jobID string) error {
	_, err := tx.ExecContext(ctx, "UPDATE workflow_steps SET state=?,updated_at=? WHERE job_id=? AND state NOT IN (?,?,?)", StepCancelled, now(), jobID, StepCompleted, StepSuperseded, StepCancelled)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE step_attempts SET status=?,ended_at=?,diagnostic=? WHERE step_id IN (SELECT id FROM workflow_steps WHERE job_id=?) AND status=?", StepCancelled, now(), "cancelled by user", jobID, StepRunning)
	return err
}

// Resolve input refs remains intentionally narrow: no path or content is
// accepted from the executor, only already registered artifact IDs and digests.
func validStepKey(v string) bool { return v != "" && !strings.ContainsAny(v, "\x00/\\") }
