package runner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/gateway"
	"chunsu/internal/gmail"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

func (r *Runner) Publish(ctx context.Context, jobID string) (Outcome, error) {
	out := Outcome{JobID: jobID}
	j, err := r.Store.Job(ctx, jobID)
	if err != nil {
		return out, err
	}
	out.AttemptID = j.CurrentAttempt
	out.Status = j.Status
	if j.Status == store.Running || j.Status == store.Cancelled || j.Status == store.Retiring || j.Status == store.Purged {
		return out, errors.New("this job is not eligible for publication")
	}
	artifacts, err := r.Store.Artifacts(ctx, jobID)
	if err != nil {
		return out, err
	}
	var raw []byte
	var manifest workgroup.Package
	var bundle workgroup.Bundle
	var generated executor.Result
	var available *store.Artifact
	observed := map[string]bool{}
	for _, art := range artifacts {
		if art.AttemptID != j.CurrentAttempt {
			continue
		}
		b, e := r.Store.ReadArtifact(art, r.Config.Limits.MaxArtifactBytes)
		if e != nil {
			return out, e
		}
		switch art.Kind {
		case "report_markdown":
			out.ReportPath = filepath.Join(r.Store.Root, art.Path)
			copy := art
			available = &copy
		case "executor_result":
			if e = mail.Decode(b, &generated); e != nil {
				return out, e
			}
		case "raw_result":
			raw = b
		case "package_manifest":
			if e = mail.Decode(b, &manifest); e != nil {
				return out, e
			}
		case "workgroup_bundle":
			if e = mail.Decode(b, &bundle); e != nil {
				return out, e
			}
		case "source_lookup":
			var evidence gateway.Evidence
			if e = mail.Decode(b, &evidence); e != nil {
				return out, e
			}
			if evidence.Result.Source != nil && evidence.Result.Source.ID == evidence.SourceID {
				observed[evidence.SourceID] = true
			}
		}
	}
	publicationWait := j.Status == store.WaitingInput && (j.Diagnostic == store.PublicationRequired || j.Diagnostic == store.RecoveryRequired)
	existingPublication := out.ReportPath != "" && (j.Status == store.Completed || j.Status == store.Partial)
	if out.ReportPath != "" && !publicationWait && !existingPublication {
		return out, nil
	}
	if !publicationWait && !existingPublication {
		return out, errors.New("job does not have a recoverable presentation checkpoint")
	}
	if generated.Outcome != "generated" || generated.ExitCode != 0 {
		return out, errors.New("executor success is not established by preserved evidence")
	}
	if len(raw) == 0 || manifest.AttemptID != j.CurrentAttempt {
		return out, errors.New("preserved output and manifest are required for publication recovery")
	}
	bundleBytes, _ := json.Marshal(bundle)
	if files.Digest(bundleBytes) != manifest.WorkgroupDigest {
		return out, errors.New("preserved workgroup version differs from the manifest")
	}
	if err = verifyManifestSkill(j, manifest, bundle); err != nil {
		return out, err
	}
	inputArt, err := r.Store.InputArtifact(ctx, j)
	if err != nil {
		return out, err
	}
	if inputArt.Digest != manifest.InputDigest {
		return out, errors.New("input differs from the preserved manifest")
	}
	input, err := r.Store.ReadArtifact(inputArt, r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return out, err
	}
	if manifest.Workgroup == "jira-report" {
		return r.publishJira(ctx, out, j, manifest, bundle, raw, input, available, publicationWait)
	}
	snapshot, err := mail.ParseSnapshot(input, r.Config.Limits)
	if err != nil {
		return out, err
	}
	recovered, e := r.collectLookups(ctx, j, store.Attempt{ID: j.CurrentAttempt, JobID: j.ID})
	if e != nil {
		return out, e
	}
	for id := range recovered {
		observed[id] = true
	}
	report, validation, err := mail.ValidateReport(raw, bundle.Schema, snapshot, manifest.Mode, observed)
	if err != nil {
		return out, err
	}
	var art store.Artifact
	if available != nil {
		art = *available
	} else {
		art, err = r.savePresentation(ctx, j.ID, j.CurrentAttempt, report, validation, snapshot)
		if err != nil {
			return out, err
		}
	}
	if publicationWait {
		if err = r.Store.CompletePublication(ctx, j.ID, j.CurrentAttempt, validation.OperationalStatus, art.ID); err != nil {
			return out, err
		}
	}
	out.Status = validation.OperationalStatus
	out.ReportPath = filepath.Join(r.Store.Root, art.Path)
	if snapshot.Origin != nil && j.CanAdvanceCoverage() && (out.Status == store.Completed || out.Status == store.Partial) {
		err = r.Store.RecordCoverage(ctx, snapshot.Origin.ConnectionID, j.ID, gmail.CoverageForReport(snapshot, report))
	}
	return out, err
}

func verifyManifestSkill(j store.Job, manifest workgroup.Package, bundle workgroup.Bundle) error {
	// Version-one attempts predate explicit Skill delivery. They remain readable
	// for mail publication recovery, but cannot be treated as Jira attempts.
	if manifest.Version == 1 {
		if manifest.Workgroup != "" && manifest.Workgroup != mail.Workgroup {
			return errors.New("legacy manifest has an unsupported workgroup")
		}
		return nil
	}
	legacyMailBundle := manifest.Version == workgroup.BundleVersion && j.Workgroup == mail.Workgroup && manifest.Workgroup == mail.Workgroup && bundle.Version == 1 && bundle.Workgroup == ""
	if manifest.Workgroup != j.Workgroup || (!legacyMailBundle && bundle.Workgroup != j.Workgroup) {
		return errors.New("preserved workgroup differs from the job manifest")
	}
	skill, err := bundle.SelectedSkill()
	if err != nil {
		return err
	}
	if skill.Name != manifest.Skill.Name || skill.Description != manifest.Skill.Description || files.Digest([]byte(skill.Markdown)) != manifest.Skill.Digest {
		return errors.New("preserved Skill differs from the attempt manifest")
	}
	return nil
}

func (r *Runner) publishJira(ctx context.Context, out Outcome, j store.Job, manifest workgroup.Package, bundle workgroup.Bundle, raw, input []byte, available *store.Artifact, publicationWait bool) (Outcome, error) {
	reportInput, err := jira.ParseReportInput(input)
	if err != nil {
		return out, err
	}
	observed, err := r.collectLookups(ctx, j, store.Attempt{ID: j.CurrentAttempt, JobID: j.ID})
	if err != nil {
		return out, err
	}
	ids, err := gateway.JiraSourceIDs(input)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		if !observed[id] {
			return out, errors.New(requiredSourceLookupsMissing)
		}
	}
	report, validation, err := jira.ValidateReport(raw, bundle.Schema, reportInput, observed)
	if err != nil {
		return out, err
	}
	var art store.Artifact
	if available != nil {
		art = *available
	} else {
		index, e := jira.BuildSourceIndex(reportInput)
		if e != nil {
			return out, e
		}
		data, e := json.MarshalIndent(index, "", "  ")
		if e != nil {
			return out, e
		}
		sources, e := r.Store.SaveArtifact(ctx, j.ID, j.CurrentAttempt, "report_sources", data, r.Config.Limits.MaxArtifactBytes)
		if e != nil {
			return out, e
		}
		art, err = r.Store.SaveArtifact(ctx, j.ID, j.CurrentAttempt, "report_markdown", jira.RenderMarkdown(report, validation, index, reportInput.Policy.TodoStatusID, filepath.Base(sources.Path)), r.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return out, err
		}
	}
	if publicationWait {
		if err = r.Store.CompletePublication(ctx, j.ID, j.CurrentAttempt, validation.OperationalStatus, art.ID); err != nil {
			return out, err
		}
	}
	out.Status = validation.OperationalStatus
	out.ReportPath = filepath.Join(r.Store.Root, art.Path)
	return out, nil
}

func (r *Runner) savePresentation(ctx context.Context, jobID, attemptID string, report mail.Report, validation mail.Validation, snapshot mail.Snapshot) (store.Artifact, error) {
	sources, err := r.Store.SaveArtifact(ctx, jobID, attemptID, mail.SourcesArtifactKind, mail.RenderSources(report, snapshot), r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return store.Artifact{}, err
	}
	return r.Store.SaveArtifact(ctx, jobID, attemptID, "report_markdown", mail.RenderWithSources(report, validation, snapshot, filepath.Base(sources.Path)), r.Config.Limits.MaxArtifactBytes)
}
