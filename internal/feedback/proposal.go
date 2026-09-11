package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"chunsu/internal/audit"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

type Proposal struct {
	Version         int      `json:"version"`
	Workgroup       string   `json:"workgroup,omitempty"`
	BaseDigest      string   `json:"base_digest"`
	CandidateDigest string   `json:"candidate_digest"`
	EvidenceIDs     []string `json:"evidence_ids"`
	Hypothesis      string   `json:"hypothesis"`
	RequiredChecks  []string `json:"required_checks"`
	Status          string   `json:"status"`
}

func proposalWorkgroup(p Proposal) string {
	if p.Workgroup == "" {
		return mail.Workgroup
	}
	return p.Workgroup
}

type Decision struct {
	Version        int    `json:"version"`
	ProposalID     string `json:"proposal_id"`
	Action         string `json:"action"`
	Actor          string `json:"actor"`
	ActorKind      string `json:"actor_kind"`
	Reason         string `json:"reason"`
	ComparisonID   string `json:"comparison_id"`
	AssessmentID   string `json:"assessment_id,omitempty"`
	PreviousDigest string `json:"previous_digest"`
	SelectedDigest string `json:"selected_digest"`
	At             string `json:"at"`
}

func (s Service) Propose(ctx context.Context, bundle workgroup.Bundle, evidence []string, hypothesis string, checks []string) (store.Record, error) {
	if len(evidence) == 0 || !mail.Nonempty(hypothesis) || len(checks) == 0 {
		return store.Record{}, errors.New("proposal needs evidence, hypothesis and required checks")
	}
	seenChecks := map[string]bool{}
	for _, check := range checks {
		if !mail.Nonempty(check) || seenChecks[check] {
			return store.Record{}, errors.New("required checks must be nonempty and unique")
		}
		seenChecks[check] = true
	}
	for _, id := range evidence {
		if _, err := s.Store.Record(ctx, id); err != nil {
			return store.Record{}, err
		}
	}
	group := bundle.Workgroup
	if group == "" {
		group = mail.Workgroup
	}
	_, base, err := workgroup.ActiveFor(s.Store.Root, group, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, err
	}
	candidate, err := workgroup.Put(s.Store.Root, bundle)
	if err != nil {
		return store.Record{}, err
	}
	if candidate == base {
		return store.Record{}, errors.New("candidate does not change the active bundle")
	}
	return s.Store.PutRecord(ctx, "proposal", "", Proposal{Version: Version, Workgroup: group, BaseDigest: base, CandidateDigest: candidate, EvidenceIDs: evidence, Hypothesis: hypothesis, RequiredChecks: checks, Status: "candidate_only"}, s.Config.Limits.MaxArtifactBytes)
}

func (s Service) Diff(ctx context.Context, id string) (map[string]any, error) {
	var p Proposal
	if err := s.load(ctx, id, "proposal", &p); err != nil {
		return nil, err
	}
	group := proposalWorkgroup(p)
	a, err := workgroup.LoadFor(s.Store.Root, group, p.BaseDigest, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return nil, err
	}
	b, err := workgroup.LoadFor(s.Store.Root, group, p.CandidateDigest, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return nil, err
	}
	return map[string]any{"proposal": p, "workgroup": group, "before": a, "after": b, "activation": "unchanged until an explicit adoption command"}, nil
}

func (s Service) Decide(ctx context.Context, id, action, actor, actorKind, reason, comparisonID string, options ...ReleaseOptions) (store.Record, error) {
	if !mail.Nonempty(actor) || !mail.Nonempty(reason) || (actorKind != "human" && actorKind != "validation") {
		return store.Record{}, errors.New("an explicit human or isolated validation decision needs an actor and reason")
	}
	if (action == "adopt" || action == "rollback") && actorKind != "human" {
		return store.Record{}, errors.New("only an explicit human decision may change active controls")
	}
	var p Proposal
	if err := s.load(ctx, id, "proposal", &p); err != nil {
		return store.Record{}, err
	}
	group := proposalWorkgroup(p)
	_, active, err := workgroup.ActiveFor(s.Store.Root, group, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, err
	}
	selected := active
	assessmentID := ""
	switch action {
	case "adopt":
		if active != p.BaseDigest {
			return store.Record{}, errors.New("active controls changed since this proposal; review a new candidate against the active version")
		}
		var comparison Comparison
		if err = s.load(ctx, comparisonID, "comparison", &comparison); err != nil {
			return store.Record{}, err
		}
		hasBase, hasCandidate := false, false
		for _, r := range comparison.Runs {
			if r.Job.Workgroup != group {
				return store.Record{}, errors.New("comparison includes a different workgroup")
			}
			if r.Manifest != nil {
				hasBase = hasBase || r.Manifest.WorkgroupDigest == p.BaseDigest
				hasCandidate = hasCandidate || r.Manifest.WorkgroupDigest == p.CandidateDigest
			}
		}
		if !hasBase || !hasCandidate {
			return store.Record{}, errors.New("comparison does not cover this baseline and candidate")
		}
		if !comparison.Comparable {
			return store.Record{}, errors.New("comparison is not comparable; resolve its limitations before adoption")
		}
		if len(options) != 1 {
			return store.Record{}, errors.New("adoption requires an explicit release policy and selected check results")
		}
		assessment, readiness, e := s.Assess(ctx, id, comparisonID, options[0])
		if e != nil {
			return store.Record{}, e
		}
		if !readiness.Eligible {
			return assessment, errors.New("candidate does not meet the selected release policy; inspect assessment " + assessment.ID)
		}
		assessmentID = assessment.ID
		selected = p.CandidateDigest
	case "rollback":
		if active != p.CandidateDigest {
			return store.Record{}, errors.New("this candidate is not the active version")
		}
		selected = p.BaseDigest
	case "reject":
	default:
		return store.Record{}, errors.New("action must be adopt, reject or rollback")
	}
	if _, err = workgroup.LoadFor(s.Store.Root, group, selected, s.Config.Limits.MaxArtifactBytes); err != nil {
		return store.Record{}, err
	}
	decision := Decision{Version: Version, ProposalID: id, Action: action, Actor: actor, ActorKind: actorKind, Reason: reason, ComparisonID: comparisonID, AssessmentID: assessmentID, PreviousDigest: active, SelectedDigest: selected, At: time.Now().UTC().Format(time.RFC3339Nano)}
	record, err := s.Store.PutRecord(ctx, "decision", id, decision, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return record, err
	}
	if action == "reject" {
		return record, nil
	}
	// The immutable decision is a selection request. The active pointer is the
	// authority for whether that request actually took effect after a crash.
	data, _ := json.Marshal(workgroup.Selection{Digest: selected, Reason: reason, DecisionID: record.ID})
	path, err := workgroup.ActivePathFor(group)
	if err != nil {
		return record, err
	}
	if err = files.Write(s.Store.Root, path, data, true); err != nil {
		return record, err
	}
	return record, audit.Record(s.Store.Root, "control.selected", record.ID, selected)
}
