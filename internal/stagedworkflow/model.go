package stagedworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"chunsu/internal/codereview"
	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/ops"
	"chunsu/internal/workgroup"
)

func buildEvidence(raw RawBundle, proposed []SourceFacts, limits config.Limits) ([]byte, error) {
	if len(proposed) != len(raw.Sources) {
		return nil, errors.New("refinement did not cover every collected source")
	}
	byID := map[string]SourceFacts{}
	for _, source := range proposed {
		if source.SourceID == "" || byID[source.SourceID].SourceID != "" {
			return nil, errors.New("refinement names a duplicate or empty source")
		}
		byID[source.SourceID] = source
	}
	rawData, _ := json.Marshal(raw)
	evidence := EvidenceBundle{Version: 1, ProjectionVersion: raw.ProjectionVersion, Workgroup: raw.Workgroup, InputDigest: raw.InputDigest, RawBundleDigest: files.Digest(rawData), PinnedMetadata: raw.PinnedMetadata, Sources: []SourceFacts{}, Gaps: append([]string{}, raw.Gaps...)}
	for _, source := range raw.Sources {
		facts, ok := byID[source.ID]
		if !ok {
			return nil, fmt.Errorf("refinement omitted source %s", source.ID)
		}
		facts.SourceDigest = source.Digest
		facts.URI = source.URI
		facts.Omissions = append(append([]string{}, source.Omissions...), facts.Omissions...)
		evidence.Sources = append(evidence.Sources, facts)
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	if _, err = validateEvidence(raw, data, limits); err != nil {
		return nil, err
	}
	return data, nil
}

// authorizeModel reloads live settings immediately before every private model
// boundary. A grant for another route or source class does not transfer.
func authorizeModel(ctx context.Context, in StageInput, role string) error {
	_ = ctx
	current, err := config.Load(in.Root)
	if err != nil {
		return err
	}
	if err = ValidatePinnedRoute(role, in.PinnedModel, in.PinnedEffort, in.PinnedIdentity, current); err != nil {
		return err
	}
	if RouteIdentity(in.Config.ExecutorFor(role)) != RouteIdentity(current.ExecutorFor(role)) {
		return errors.New("staged route changed before source disclosure")
	}
	if in.Workgroup == mail.Workgroup && current.MailMode != in.PinnedMailMode {
		return errors.New("mail mode changed after staged admission")
	}
	if in.Workgroup == WebWorkgroup {
		return nil
	}
	route := current.ExecutorFor(role)
	switch in.Workgroup {
	case mail.Workgroup:
		snapshot, e := mail.ParseSnapshot(in.Snapshot, current.Limits)
		if e != nil {
			return e
		}
		if !snapshot.Synthetic && !route.LiveMailApproved {
			return errors.New("live mail disclosure is not approved for this stage route")
		}
	case "jira-report":
		report, e := jira.ParseReportInput(in.Snapshot)
		if e != nil {
			return e
		}
		if !report.Snapshot.Synthetic && (!route.LiveJiraApproved || route.LiveJiraValidationJobID == "" || route.LiveJiraPolicyDigest != report.ReportPolicyDigest) {
			return errors.New("live Jira disclosure lacks matching stage route and policy approval")
		}
	case codereview.Workgroup:
		snapshot, e := codereview.Parse(in.Snapshot, current.Limits)
		if e != nil {
			return e
		}
		if e = codereview.Authorize(snapshot, route); e != nil {
			return e
		}
	case ops.Workgroup:
		input, e := ops.ParseInput(in.Snapshot, current.Limits)
		if e != nil {
			return e
		}
		return ops.AuthorizeInput(input, route)
	default:
		return errors.New("no disclosure adapter for staged source")
	}
	return nil
}

func pinnedBundle(in StageInput) (workgroup.Bundle, error) {
	if in.Workgroup == WebWorkgroup {
		return workgroup.Bundle{}, errors.New("web workflow has no report bundle")
	}
	if !files.ValidDigest(in.PinnedBundleDigest) {
		return workgroup.Bundle{}, errors.New("staged workgroup bundle digest is missing")
	}
	if in.PinnedSkillScope != nil {
		bundle, err := workgroup.LoadFor(in.Root, in.Workgroup, in.PinnedBundleDigest, in.Config.Limits.MaxArtifactBytes)
		if err == nil {
			err = workgroup.ValidateScopeBundle(bundle, *in.PinnedSkillScope)
		}
		return bundle, err
	}
	active, digest, err := workgroup.ActiveFor(in.Root, in.Workgroup, in.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return workgroup.Bundle{}, err
	}
	if digest != in.PinnedBundleDigest {
		return workgroup.Bundle{}, errors.New("active workgroup bundle changed after staged admission")
	}
	if active.Composition != nil {
		return workgroup.Bundle{}, errors.New("scoped composition requires an explicit admitted application scope")
	}
	return active, nil
}
