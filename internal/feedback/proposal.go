package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

type Proposal struct {
	Version         int      `json:"version"`
	BaseDigest      string   `json:"base_digest"`
	CandidateDigest string   `json:"candidate_digest"`
	EvidenceIDs     []string `json:"evidence_ids"`
	Hypothesis      string   `json:"hypothesis"`
	RequiredChecks  []string `json:"required_checks"`
	Status          string   `json:"status"`
}
type Decision struct {
	Version        int    `json:"version"`
	ProposalID     string `json:"proposal_id"`
	Action         string `json:"action"`
	Actor          string `json:"actor"`
	ActorKind      string `json:"actor_kind"`
	Reason         string `json:"reason"`
	ComparisonID   string `json:"comparison_id"`
	PreviousDigest string `json:"previous_digest"`
	SelectedDigest string `json:"selected_digest"`
	At             string `json:"at"`
}

func (s Service) Propose(ctx context.Context, bundle workgroup.Bundle, evidence []string, hypothesis string, checks []string) (store.Record, error) {
	if len(evidence) == 0 || !mail.Nonempty(hypothesis) || len(checks) == 0 {
		return store.Record{}, errors.New("proposal needs evidence, hypothesis and required checks")
	}
	for _, id := range evidence {
		if _, err := s.Store.Record(ctx, id); err != nil {
			return store.Record{}, err
		}
	}
	_, base, err := workgroup.Active(s.Store.Root, s.Config.Limits.MaxArtifactBytes)
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
	return s.Store.PutRecord(ctx, "proposal", "", Proposal{Version: Version, BaseDigest: base, CandidateDigest: candidate, EvidenceIDs: evidence, Hypothesis: hypothesis, RequiredChecks: checks, Status: "candidate_only"}, s.Config.Limits.MaxArtifactBytes)
}

func (s Service) Diff(ctx context.Context, id string) (map[string]any, error) {
	var p Proposal
	if err := s.load(ctx, id, "proposal", &p); err != nil {
		return nil, err
	}
	a, err := workgroup.Load(s.Store.Root, p.BaseDigest, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return nil, err
	}
	b, err := workgroup.Load(s.Store.Root, p.CandidateDigest, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return nil, err
	}
	return map[string]any{"proposal": p, "before": a, "after": b, "activation": "unchanged until an explicit adoption command"}, nil
}

func (s Service) Decide(ctx context.Context, id, action, actor, actorKind, reason, comparisonID string) (store.Record, error) {
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
	_, active, err := workgroup.Active(s.Store.Root, s.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Record{}, err
	}
	selected := active
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
	if _, err = workgroup.Load(s.Store.Root, selected, s.Config.Limits.MaxArtifactBytes); err != nil {
		return store.Record{}, err
	}
	decision := Decision{Version: Version, ProposalID: id, Action: action, Actor: actor, ActorKind: actorKind, Reason: reason, ComparisonID: comparisonID, PreviousDigest: active, SelectedDigest: selected, At: time.Now().UTC().Format(time.RFC3339Nano)}
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
	return record, files.Write(s.Store.Root, workgroup.ActivePath, data, true)
}
