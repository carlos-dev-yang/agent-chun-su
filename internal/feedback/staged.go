package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/ops"
	"chunsu/internal/runtimeenv"
	"chunsu/internal/stagedworkflow"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

// FlowRunEvidence is a projection of real staged rows and immutable artifacts,
// not a legacy attempt or fabricated package manifest. Its evaluable anchor is
// the validate StepAttempt; synthesis and refinement retain their own identities.
type FlowRunEvidence struct {
	Version        int                   `json:"version"`
	Workflow       store.WorkflowJob     `json:"workflow"`
	Steps          []store.Step          `json:"steps"`
	Attempts       []store.StepAttempt   `json:"attempts"`
	Input          store.Artifact        `json:"input"`
	Outputs        []store.Artifact      `json:"outputs"`
	Models         []FlowModelEvidence   `json:"models"`
	HostRefinement *FlowHostEvidence     `json:"host_refinement,omitempty"`
	BundleDigest   string                `json:"bundle_digest"`
	SkillScope     *workgroup.SkillScope `json:"skill_scope,omitempty"`
	InputDigest    string                `json:"input_digest"`
	RequestDigest  string                `json:"request_digest"`
	Mode           string                `json:"mode"`
	Limits         config.Limits         `json:"limits"`
	ReportStatus   string                `json:"report_status,omitempty"`
	RevisionDigest string                `json:"revision_digest,omitempty"`
	SelectedSkill  string                `json:"selected_skill_digest,omitempty"`
	AppliedSkill   string                `json:"applied_skill_digest,omitempty"`
}

type FlowModelEvidence struct {
	Role      string                   `json:"role"`
	StepID    string                   `json:"step_id"`
	AttemptID string                   `json:"attempt_id"`
	Receipt   store.Artifact           `json:"receipt"`
	Response  store.Artifact           `json:"response"`
	AuditHash string                   `json:"audit_digest"`
	Audit     executor.StructuredAudit `json:"audit"`
}

type FlowHostEvidence struct {
	StepID    string                               `json:"step_id"`
	AttemptID string                               `json:"attempt_id"`
	Receipt   store.Artifact                       `json:"receipt"`
	Content   stagedworkflow.HostRefinementReceipt `json:"content"`
}

func selectedAttempt(r RunEvidence) string {
	if r.Flow != nil {
		return r.SelectedAttemptID
	}
	return r.Job.CurrentAttempt
}

func bundlePin(r RunEvidence) (string, *workgroup.SkillScope, bool) {
	if r.Flow != nil {
		return r.Flow.BundleDigest, r.Flow.SkillScope, r.Flow.RevisionDigest != ""
	}
	if r.Manifest != nil {
		return r.Manifest.WorkgroupDigest, r.Manifest.SkillScope, true
	}
	return "", nil, false
}

func reportStatus(r RunEvidence) string {
	if r.Flow != nil {
		return r.Flow.ReportStatus
	}
	return r.Job.Status
}

func sameScope(a, b *workgroup.SkillScope) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func equalJSON(a, b any) bool {
	x, err := json.Marshal(a)
	if err != nil {
		return false
	}
	y, err := json.Marshal(b)
	return err == nil && string(x) == string(y)
}

func (s Service) inspectFlow(ctx context.Context, r *RunEvidence, workflow store.WorkflowJob) error {
	f := &FlowRunEvidence{Version: Version, Workflow: workflow, Attempts: []store.StepAttempt{}, Outputs: []store.Artifact{}, Models: []FlowModelEvidence{}}
	r.Flow = f
	if workflow.WorkflowVersion != stagedworkflow.Version || workflow.RequestRevision <= 0 {
		return errors.New("FLOW review requires the supported workflow and a durable request revision")
	}
	var scope struct {
		Workgroup    string                `json:"workgroup"`
		InputDigest  string                `json:"input_digest"`
		SourceIDs    []string              `json:"source_ids"`
		BundleDigest string                `json:"bundle_digest"`
		MailMode     string                `json:"mail_mode"`
		SkillScope   *workgroup.SkillScope `json:"skill_scope"`
	}
	if err := json.Unmarshal(workflow.SourceScope, &scope); err != nil {
		return err
	}
	if scope.Workgroup != r.Job.Workgroup || !files.ValidDigest(scope.BundleDigest) || !files.ValidDigest(scope.InputDigest) {
		return errors.New("FLOW source and bundle pins are incomplete")
	}
	f.BundleDigest, f.SkillScope, f.InputDigest, f.Mode = scope.BundleDigest, scope.SkillScope, scope.InputDigest, scope.MailMode
	var objective string
	if err := json.Unmarshal(workflow.OriginalRequest, &objective); err != nil || strings.TrimSpace(objective) == "" {
		return errors.New("FLOW objective is unavailable")
	}
	f.RequestDigest = files.Digest(workflow.OriginalRequest)
	if err := mail.Decode(workflow.Budget, &f.Limits); err != nil {
		return err
	}
	retained := s.Config
	retained.Limits = f.Limits
	if err := retained.Validate(); err != nil {
		return err
	}
	bundle, err := workgroup.LoadFor(s.Store.Root, r.Job.Workgroup, f.BundleDigest, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return err
	}
	if f.SkillScope != nil {
		if err = workgroup.ValidateScopeBundle(bundle, *f.SkillScope); err != nil {
			return err
		}
	} else if bundle.Composition != nil {
		return errors.New("FLOW composition lacks its admitted application scope")
	}
	var requestPin struct {
		Scope  *workgroup.SkillScope `json:"skill_scope"`
		Digest string                `json:"skill_bundle_digest"`
	}
	if err = json.Unmarshal(r.Job.Request, &requestPin); err != nil {
		return err
	}
	if !sameScope(requestPin.Scope, f.SkillScope) || (f.SkillScope != nil && requestPin.Digest != f.BundleDigest) {
		return errors.New("FLOW request and workflow Skill pins differ")
	}
	if f.Input, err = s.Store.InputArtifact(ctx, r.Job); err != nil {
		return err
	}
	input, err := s.Store.ReadArtifact(f.Input, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return err
	}
	if f.Input.Digest != f.InputDigest {
		return errors.New("FLOW input differs from its admitted source digest")
	}
	if f.Steps, err = s.Store.Steps(ctx, r.Job.ID); err != nil {
		return err
	}
	stages := []string{stagedworkflow.Collect, stagedworkflow.Refine, stagedworkflow.Synthesize, stagedworkflow.Validate, stagedworkflow.Deliver}
	if len(f.Steps) != len(stages) {
		return errors.New("FLOW review requires the exact bounded report workflow")
	}
	current := make([]store.StepAttempt, len(stages))
	ready := true
	for i, step := range f.Steps {
		if step.JobID != r.Job.ID || step.Key != stages[i] || step.Stage != stages[i] || step.Ordinal != i+1 || step.InstructionVersion != workflow.WorkflowVersion || step.ExpectedOutputSchema != step.Stage+"-v1" {
			return errors.New("FLOW stage identity or instruction version differs from its contract")
		}
		dependencies := []string{}
		if i > 0 {
			dependencies = append(dependencies, stages[i-1])
		}
		if !slices.Equal(dependencies, step.DependsOn) {
			return errors.New("FLOW stage dependencies differ from its contract")
		}
		attempts, e := s.Store.StepAttempts(ctx, step.ID)
		if e != nil {
			return e
		}
		f.Attempts = append(f.Attempts, attempts...)
		for _, attempt := range attempts {
			if attempt.ID == step.CurrentAttemptID {
				current[i] = attempt
			}
		}
		if step.State == store.StepRunning {
			return errors.New("review requires a non-running FLOW job")
		}
		if i < len(stages)-1 && (step.State != store.StepCompleted || current[i].ID == "" || current[i].Status != store.StepCompleted || step.OutputArtifactID == "") {
			ready = false
		}
	}
	if !ready {
		r.Gaps = append(r.Gaps, "FLOW has no current complete collect/refine/synthesis/validation chain")
		return nil
	}
	data := make([][]byte, len(stages)-1)
	previous := f.Input
	for i, step := range f.Steps[:len(stages)-1] {
		attempt := current[i]
		if attempt.StepID != step.ID || attempt.ExecutorModel != step.ExecutorModel || attempt.ExecutorEffort != step.ExecutorEffort || attempt.ExecutorIdentity != step.ExecutorIdentity || attempt.InstructionVersion != step.InstructionVersion || attempt.OutputArtifactID != step.OutputArtifactID {
			return errors.New("FLOW current attempt differs from its stage pin")
		}
		expectedInputs := []store.PinnedArtifactRef{{ArtifactID: previous.ID, Digest: previous.Digest}}
		if !equalJSON(attempt.Inputs, expectedInputs) || !equalJSON(step.InputArtifacts, expectedInputs) {
			return errors.New("FLOW current artifact ancestry changed")
		}
		kind := []string{"staged_raw_evidence", "staged_evidence", "staged_synthesis", "staged_validated"}[i]
		artifact, e := selectedArtifact(r.Artifacts, step.OutputArtifactID, attempt.ID, kind)
		if e != nil {
			return e
		}
		if data[i], err = s.Store.ReadArtifact(artifact, s.Config.Limits.MaxArtifactBytes); err != nil {
			return err
		}
		f.Outputs = append(f.Outputs, artifact)
		previous = artifact
	}
	contracts, err := stagedworkflow.ModelContracts(r.Job.Workgroup, objective, data[0], data[1], f.Limits, bundle)
	if err != nil {
		return err
	}
	modelResponses := make([][]byte, 2)
	roles := []string{config.RoleRefinement, config.RoleSynthesis}
	if r.Job.Workgroup == ops.Workgroup {
		if f.Steps[0].ExecutorModel != "" || f.Steps[0].ExecutorEffort != "" || f.Steps[0].ExecutorIdentity != "host:collect:"+stagedworkflow.Version || f.Steps[1].ExecutorModel != "" || f.Steps[1].ExecutorEffort != "" || f.Steps[1].ExecutorIdentity != stagedworkflow.OpsHostRefinementIdentity {
			return errors.New("team-ops collection/refinement must retain their exact deterministic host pins")
		}
		var raw stagedworkflow.RawBundle
		if err = mail.Decode(data[0], &raw); err != nil {
			return err
		}
		_, expected, e := stagedworkflow.ProjectOpsEvidence(raw, f.Limits)
		if e != nil {
			return e
		}
		proof, e := uniqueAttemptArtifact(r.Artifacts, current[1].ID, "staged_host_refinement")
		if e != nil {
			return e
		}
		content, e := s.Store.ReadArtifact(proof, s.Config.Limits.MaxArtifactBytes)
		if e != nil {
			return e
		}
		var actual stagedworkflow.HostRefinementReceipt
		if e = mail.Decode(content, &actual); e != nil || !equalJSON(actual, expected) {
			return errors.New("team-ops host refinement receipt differs from its deterministic projection")
		}
		for _, artifact := range r.Artifacts {
			if artifact.AttemptID == current[1].ID && (artifact.Kind == "staged_model_response" || artifact.Kind == "staged_executor_receipt") {
				return errors.New("team-ops host refinement cannot claim an AI executor receipt")
			}
		}
		f.HostRefinement = &FlowHostEvidence{StepID: f.Steps[1].ID, AttemptID: current[1].ID, Receipt: proof, Content: actual}
		roles = []string{config.RoleSynthesis}
	}
	for _, role := range roles {
		index := 1
		if role == config.RoleSynthesis {
			index = 2
		}
		model, response, e := s.flowModel(r.Job.ID, f.Steps[index], current[index], role, r.Artifacts, contracts[role])
		if e != nil {
			return e
		}
		f.Models = append(f.Models, model)
		modelResponses[index-1] = response
	}
	validated, err := stagedworkflow.VerifyResultEvidence(r.Job.Workgroup, input, f.Limits, bundle, f.Mode, data[0], data[1], data[2], data[3], modelResponses[0], modelResponses[1])
	if err != nil {
		return err
	}
	if !slices.Equal(scope.SourceIDs, validated.SourceIDs) {
		return errors.New("FLOW validation changed the admitted source identifiers")
	}
	var synthesis stagedworkflow.Synthesis
	if err = mail.Decode(data[2], &synthesis); err != nil {
		return err
	}
	synthesisModel := f.Models[len(f.Models)-1]
	if synthesisModel.Audit.SkillDigest != synthesis.AppliedSkillDigest {
		return errors.New("FLOW synthesis executor did not receive its pinned adapted Skill")
	}
	f.SelectedSkill, f.AppliedSkill = synthesis.SelectedSkillDigest, synthesis.AppliedSkillDigest
	f.ReportStatus = validated.Status
	r.SelectedAttemptID = current[3].ID
	r.ExecutorResult = &f.Models[len(f.Models)-1].Audit.Result
	// Delivery state is intentionally not quality provenance. Request revision,
	// all attempts (including failed retries), exact upstream artifacts and both
	// model audit hashes are included; a changed chain needs fresh comparison.
	qualitySteps := append([]store.Step(nil), f.Steps[:len(stages)-1]...)
	qualityAttempts := []store.StepAttempt{}
	for _, attempt := range f.Attempts {
		if attempt.StepID != f.Steps[len(stages)-1].ID {
			qualityAttempts = append(qualityAttempts, attempt)
		}
	}
	revision, err := json.Marshal(struct {
		WorkflowVersion                              string
		RequestRevision                              int64
		Request, Scope, Budget, Criteria, JobRequest json.RawMessage
		Steps                                        []store.Step
		Attempts                                     []store.StepAttempt
		Input                                        store.Artifact
		Outputs                                      []store.Artifact
		Models                                       []FlowModelEvidence
		HostRefinement                               *FlowHostEvidence `json:"HostRefinement,omitempty"`
	}{workflow.WorkflowVersion, workflow.RequestRevision, workflow.OriginalRequest, workflow.SourceScope, workflow.Budget, workflow.CompletionCriteria, r.Job.Request, qualitySteps, qualityAttempts, f.Input, f.Outputs, f.Models, f.HostRefinement})
	if err != nil {
		return err
	}
	f.RevisionDigest = files.Digest(revision)
	return nil
}

func selectedArtifact(artifacts []store.Artifact, id, attempt, kind string) (store.Artifact, error) {
	for _, artifact := range artifacts {
		if artifact.ID == id && artifact.AttemptID == attempt && artifact.Kind == kind {
			return artifact, nil
		}
	}
	return store.Artifact{}, fmt.Errorf("selected FLOW %s artifact is unavailable", kind)
}

func uniqueAttemptArtifact(artifacts []store.Artifact, attempt, kind string) (store.Artifact, error) {
	var found store.Artifact
	for _, artifact := range artifacts {
		if artifact.AttemptID == attempt && artifact.Kind == kind {
			if found.ID != "" {
				return found, errors.New("ambiguous FLOW model evidence")
			}
			found = artifact
		}
	}
	if found.ID == "" {
		return found, errors.New("FLOW lacks exact model response/audit evidence; rerun this saved input before release comparison")
	}
	return found, nil
}

func (s Service) flowModel(jobID string, step store.Step, attempt store.StepAttempt, role string, artifacts []store.Artifact, expected stagedworkflow.ModelContract) (FlowModelEvidence, []byte, error) {
	m := FlowModelEvidence{Role: role, StepID: step.ID, AttemptID: attempt.ID}
	var err error
	if m.Receipt, err = uniqueAttemptArtifact(artifacts, attempt.ID, "staged_executor_receipt"); err != nil {
		return m, nil, err
	}
	if m.Response, err = uniqueAttemptArtifact(artifacts, attempt.ID, "staged_model_response"); err != nil {
		return m, nil, err
	}
	receiptData, err := s.Store.ReadArtifact(m.Receipt, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return m, nil, err
	}
	response, err := s.Store.ReadArtifact(m.Response, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return m, nil, err
	}
	directory := filepath.Join("runs", jobID, "steps", step.ID, "attempts", attempt.ID, role)
	auditData, err := files.Read(s.Store.Root, filepath.Join(directory, "executor.json"), s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return m, nil, err
	}
	m.AuditHash = files.Digest(auditData)
	if err = mail.Decode(auditData, &m.Audit); err != nil {
		return m, nil, err
	}
	var receipt executor.Result
	if err = mail.Decode(receiptData, &receipt); err != nil {
		return m, nil, err
	}
	if !equalJSON(receipt, m.Audit.Result) || receipt.Role != role || receipt.Driver != "codex" || receipt.Outcome != "generated" || receipt.ExitCode != 0 || len(receipt.ObservedTools) != 0 || receipt.Version == "" || !files.ValidDigest(receipt.ArgumentsDigest) || m.Audit.ArgsDigest != receipt.ArgumentsDigest || m.Audit.ReplyDigest != files.Digest(response) || !files.ValidDigest(m.Audit.PromptDigest) {
		return m, nil, errors.New("FLOW model response differs from its completed tool-free execution receipt")
	}
	if m.Audit.PromptDigest != expected.PromptDigest || m.Audit.SchemaDigest != expected.SchemaDigest || m.Audit.SkillDigest != expected.SkillDigest || !files.ValidDigest(expected.PromptDigest) {
		return m, nil, errors.New("FLOW submitted prompt, schema or role Skill differs from its admitted objective and canonical source contract")
	}
	if m.Audit.Requested == nil || m.Audit.Model != step.ExecutorModel || m.Audit.Requested.Model != step.ExecutorModel || m.Audit.Requested.ReasoningEffort != step.ExecutorEffort || stagedworkflow.RouteIdentity(*m.Audit.Requested) != step.ExecutorIdentity {
		return m, nil, errors.New("FLOW model requested route does not match its admitted pin; older runs need new execution evidence")
	}
	b := receipt.Boundary
	if b == nil || b.Version != runtimeenv.PolicyVersion || b.Kind != runtimeenv.NativeRestricted || len(b.ReadRoots) != 1 || len(b.WriteRoots) != 0 || b.ToolNetwork || !filepath.IsAbs(b.ReadRoots[0]) || !strings.HasSuffix(filepath.Clean(b.ReadRoots[0]), string(filepath.Separator)+directory) {
		return m, nil, errors.New("FLOW model boundary was not restricted to its dedicated package")
	}
	args, err := json.Marshal(executor.StructuredArguments(receipt.Version, b.ReadRoots[0], *m.Audit.Requested, *b))
	if err != nil || files.Digest(args) != receipt.ArgumentsDigest {
		return m, nil, errors.New("FLOW model arguments differ from the canonical tool-free invocation")
	}
	for name, digest := range map[string]string{"SKILL.md": m.Audit.SkillDigest, "response.schema.json": m.Audit.SchemaDigest} {
		content, e := files.Read(s.Store.Root, filepath.Join(directory, name), s.Config.Limits.MaxArtifactBytes)
		if e != nil || files.Digest(content) != digest {
			return m, nil, errors.New("FLOW model instruction/schema integrity mismatch")
		}
	}
	return m, response, nil
}

func compareFlows(a, b *FlowRunEvidence) []string {
	limitations := []string{}
	if a.RevisionDigest == "" || b.RevisionDigest == "" {
		limitations = append(limitations, "FLOW results lack verified complete quality evidence")
	}
	if a.Workflow.WorkflowVersion != b.Workflow.WorkflowVersion || !equalJSON(a.Workflow.CompletionCriteria, b.Workflow.CompletionCriteria) {
		limitations = append(limitations, "FLOW contracts or completion criteria differ")
	}
	if !sameScope(a.SkillScope, b.SkillScope) {
		limitations = append(limitations, "Skill application scopes differ")
	}
	if a.InputDigest != b.InputDigest {
		limitations = append(limitations, "inputs/as-of/history differ")
	}
	if a.RequestDigest != b.RequestDigest {
		limitations = append(limitations, "human objective differs")
	}
	if a.Mode != b.Mode || !equalJSON(a.Limits, b.Limits) {
		limitations = append(limitations, "report mode or operational budgets differ")
	}
	expectedModels := 2
	if a.HostRefinement != nil && b.HostRefinement != nil {
		expectedModels = 1
		if a.HostRefinement.Content.Identity != b.HostRefinement.Content.Identity || a.HostRefinement.Content.ProjectionVersion != b.HostRefinement.Content.ProjectionVersion || a.HostRefinement.Content.CollectionVersion != b.HostRefinement.Content.CollectionVersion {
			limitations = append(limitations, "FLOW host refinement projection versions differ")
		}
	} else if (a.HostRefinement == nil) != (b.HostRefinement == nil) {
		limitations = append(limitations, "FLOW refinement engines differ")
	}
	if len(a.Models) != len(b.Models) || len(a.Models) != expectedModels {
		limitations = append(limitations, "FLOW model execution proof is incomplete")
	} else {
		for i := range a.Models {
			x, y := a.Models[i], b.Models[i]
			if x.Role != y.Role || x.Audit.Model != y.Audit.Model || x.Audit.Result.Version != y.Audit.Result.Version || x.Audit.Requested == nil || y.Audit.Requested == nil || stagedworkflow.RouteIdentity(*x.Audit.Requested) != stagedworkflow.RouteIdentity(*y.Audit.Requested) {
				limitations = append(limitations, "FLOW "+x.Role+" requested route/model/effort or runtime version differs")
			}
		}
	}
	return limitations
}
