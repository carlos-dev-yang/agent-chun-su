// Package skills manages owner-operated scoped instruction assets. The scope
// labels are organizational metadata, not a multi-user authorization system.
package skills

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"chunsu/internal/audit"
	"chunsu/internal/config"
	"chunsu/internal/feedback"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

type Service struct {
	Store  *store.Store
	Config config.Config
}

type Registration struct {
	Version           int                  `json:"version"`
	SkillDigest       string               `json:"skill_digest"`
	Scope             workgroup.SkillScope `json:"scope"`
	Name              string               `json:"name"`
	Actor             string               `json:"actor"`
	Reason            string               `json:"reason"`
	EvidenceIDs       []string             `json:"evidence_ids"`
	SanitizedForScope bool                 `json:"sanitized_for_scope"`
}

// Register preserves provenance separately from executable instructions.
// The owner's sanitization declaration is recorded, not machine-certified.
func (s Service) Register(ctx context.Context, skill workgroup.ScopedSkill, actor, reason string, evidence []string, sanitized bool) (store.Record, string, error) {
	if !mail.Nonempty(actor) || !mail.Nonempty(reason) {
		return store.Record{}, "", errors.New("Skill registration needs an owner actor and reason")
	}
	if skill.Scope.Kind == "common" && !sanitized {
		return store.Record{}, "", errors.New("common publication needs an explicit --sanitized-for-scope declaration")
	}
	for _, id := range evidence {
		record, err := s.Store.Record(ctx, id)
		if err != nil {
			return store.Record{}, "", err
		}
		if _, err = s.Store.ReadRecord(record, s.Config.Limits.MaxArtifactBytes); err != nil {
			return store.Record{}, "", err
		}
	}
	digest, err := workgroup.PutScopedSkill(s.Store.Root, skill, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, "", err
	}
	registration := Registration{Version: workgroup.ScopedSkillVersion, SkillDigest: digest, Scope: skill.Scope, Name: skill.Name, Actor: actor, Reason: reason, EvidenceIDs: evidence, SanitizedForScope: sanitized}
	record, err := s.Store.PutRecord(ctx, "skill_registration", digest, registration, s.Config.Limits.MaxArtifactBytes)
	return record, digest, err
}

func (s Service) Initialize(ctx context.Context, group string, scope workgroup.SkillScope, actor, reason string) (store.Record, error) {
	if !mail.Nonempty(actor) || !mail.Nonempty(reason) {
		return store.Record{}, errors.New("scoped baseline initialization needs an owner actor and reason")
	}
	path, err := workgroup.ScopeActivePath(group, scope)
	if err != nil {
		return store.Record{}, err
	}
	if _, err = files.Read(s.Store.Root, path, s.Config.Limits.MaxArtifactBytes); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return store.Record{}, errors.New("scoped baseline already exists; use a reviewed proposal to change it")
		}
		return store.Record{}, err
	}
	bundle, digest, err := workgroup.ActiveFor(s.Store.Root, group, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, err
	}
	if bundle.Composition != nil {
		return store.Record{}, errors.New("initialization requires the existing selected domain baseline, not a scoped composition")
	}
	decision := feedback.Decision{Version: feedback.Version, Action: "initialize_scope", Actor: actor, ActorKind: "human", Reason: reason, SelectedDigest: digest, At: time.Now().UTC().Format(time.RFC3339Nano), Scope: &scope}
	record, err := s.Store.PutRecord(ctx, "decision", "", decision, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return record, err
	}
	data, _ := json.Marshal(workgroup.Selection{Digest: digest, Reason: reason, DecisionID: record.ID})
	if err = files.Write(s.Store.Root, path, data, false); err != nil {
		return record, err
	}
	return record, audit.Record(s.Store.Root, "skill.scope_initialized", record.ID, digest)
}

type ComposeRequest struct {
	Version    int                  `json:"version"`
	Workgroup  string               `json:"workgroup"`
	Scope      workgroup.SkillScope `json:"scope"`
	Components []string             `json:"components"`
}

func (s Service) Compose(ctx context.Context, request ComposeRequest) (workgroup.Bundle, error) {
	if request.Version != workgroup.ScopedSkillVersion {
		return workgroup.Bundle{}, errors.New("composition request requires version 1")
	}
	if len(request.Components) == 0 || len(request.Components) > workgroup.MaxSkillComponents {
		return workgroup.Bundle{}, errors.New("composition requires a bounded nonempty component list")
	}
	_, base, err := workgroup.ActiveInScope(s.Store.Root, request.Workgroup, &request.Scope, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return workgroup.Bundle{}, err
	}
	for _, digest := range request.Components {
		records, err := s.Store.Records(ctx, "skill_registration", digest, 1)
		if err != nil {
			return workgroup.Bundle{}, err
		}
		if len(records) == 0 {
			return workgroup.Bundle{}, errors.New("composition component has no owner registration")
		}
		if _, err = s.Store.ReadRecord(records[0], s.Config.Limits.MaxArtifactBytes); err != nil {
			return workgroup.Bundle{}, err
		}
	}
	return workgroup.ComposeSkills(s.Store.Root, request.Workgroup, base, request.Scope, request.Components, s.Config.Limits.MaxArtifactBytes)
}

type Feedback struct {
	Version      int                       `json:"version"`
	JobID        string                    `json:"job_id"`
	Workgroup    string                    `json:"workgroup"`
	Scope        workgroup.SkillScope      `json:"scope"`
	BundleDigest string                    `json:"bundle_digest"`
	InputDigest  string                    `json:"input_digest"`
	Artifacts    []store.PinnedArtifactRef `json:"artifacts"`
	Kind         string                    `json:"kind"`
	Observation  string                    `json:"observation"`
	Correction   string                    `json:"correction,omitempty"`
	Actor        string                    `json:"actor"`
}

// RecordFeedback references the admitted instruction version and preserved
// artifacts, never embedding their raw source contents into a reusable Skill.
func (s Service) RecordFeedback(ctx context.Context, jobID, kind, observation, correction, actor string) (store.Record, error) {
	switch kind {
	case "false-positive", "omission", "severity", "owner", "duplicate", "insufficient-evidence", "useful":
	default:
		return store.Record{}, errors.New("unknown Skill feedback kind")
	}
	if !mail.Nonempty(observation) || !mail.Nonempty(actor) || (kind != "useful" && !mail.Nonempty(correction)) {
		return store.Record{}, errors.New("feedback needs an owner, observation and correction (except useful)")
	}
	job, err := s.Store.Job(ctx, jobID)
	if err != nil {
		return store.Record{}, err
	}
	var pin struct {
		Scope        *workgroup.SkillScope `json:"skill_scope"`
		BundleDigest string                `json:"skill_bundle_digest"`
	}
	if err = json.Unmarshal(job.Request, &pin); err != nil {
		return store.Record{}, err
	}
	if pin.Scope == nil {
		return store.Record{}, errors.New("feedback requires an explicitly scoped job; legacy result feedback remains available")
	}
	bundle, err := workgroup.LoadFor(s.Store.Root, job.Workgroup, pin.BundleDigest, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, err
	}
	if err = workgroup.ValidateScopeBundle(bundle, *pin.Scope); err != nil {
		return store.Record{}, err
	}
	input, err := s.Store.InputArtifact(ctx, job)
	if err != nil {
		return store.Record{}, err
	}
	if _, err = s.Store.ReadArtifact(input, s.Config.Limits.MaxArtifactBytes); err != nil {
		return store.Record{}, err
	}
	artifacts, err := s.Store.Artifacts(ctx, jobID)
	if err != nil {
		return store.Record{}, err
	}
	refs := []store.PinnedArtifactRef{}
	for _, artifact := range artifacts {
		if _, err = s.Store.ReadArtifact(artifact, s.Config.Limits.MaxArtifactBytes); err != nil {
			return store.Record{}, err
		}
		refs = append(refs, store.PinnedArtifactRef{ArtifactID: artifact.ID, Digest: artifact.Digest})
	}
	item := Feedback{Version: workgroup.ScopedSkillVersion, JobID: jobID, Workgroup: job.Workgroup, Scope: *pin.Scope, BundleDigest: pin.BundleDigest, InputDigest: input.Digest, Artifacts: refs, Kind: kind, Observation: observation, Correction: correction, Actor: actor}
	return s.Store.PutRecord(ctx, "skill_feedback", jobID, item, s.Config.Limits.MaxArtifactBytes)
}
