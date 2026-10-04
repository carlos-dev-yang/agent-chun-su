package ops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
)

type WorkStatus struct {
	WorkID                string `json:"work_id"`
	State                 string `json:"state"`
	OriginalDue           string `json:"original_due"`
	CurrentDue            string `json:"current_due"`
	OverdueOriginal       bool   `json:"overdue_original"`
	OverdueCurrent        bool   `json:"overdue_current"`
	ScheduleChanged       bool   `json:"schedule_changed"`
	WaitingCheckDue       bool   `json:"waiting_check_due"`
	CompletedLateOriginal bool   `json:"completed_late_original"`
	CompletedLateCurrent  bool   `json:"completed_late_current"`
}

type ReleaseStatus struct {
	ReleaseID            string `json:"release_id"`
	WorkID               string `json:"work_id"`
	Commitment           string `json:"commitment"`
	Readiness            string `json:"readiness"`
	Deployment           string `json:"deployment"`
	DeploymentEvidenceAt string `json:"deployment_evidence_at"`
	ReadyDeadlinePassed  bool   `json:"ready_deadline_passed"`
	DeployDeadlinePassed bool   `json:"deploy_deadline_passed"`
	Disposition          string `json:"disposition"`
}

type Disposition struct {
	SourceID string   `json:"source_id"`
	Outcome  string   `json:"outcome"`
	RiskKeys []string `json:"risk_keys"`
	Reason   string   `json:"reason"`
}

type Report struct {
	Version        int             `json:"version"`
	Scope          Scope           `json:"scope"`
	LedgerRevision int64           `json:"ledger_revision"`
	StateDigest    string          `json:"state_digest"`
	AsOf           string          `json:"as_of"`
	Summary        string          `json:"summary"`
	WorkStatus     []WorkStatus    `json:"work_status"`
	ReleaseStatus  []ReleaseStatus `json:"release_status"`
	Risks          []Risk          `json:"risks"`
	Dispositions   []Disposition   `json:"dispositions"`
	Limitations    []string        `json:"limitations"`
}

type Validation struct {
	OperationalStatus string   `json:"operational_status"`
	Gaps              []string `json:"gaps"`
	Checks            []string `json:"checks"`
}

func overdue(asOf time.Time, due string) bool {
	if due == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, due)
	return err == nil && asOf.After(t)
}

func ProjectStatus(in Input) ([]WorkStatus, []ReleaseStatus) {
	asOf, _ := time.Parse(time.RFC3339Nano, in.AsOf)
	work := []WorkStatus{}
	byID := map[string]WorkItem{}
	for _, item := range in.WorkItems {
		byID[item.ID] = item
		open := !oneOf(item.State, Completed, Cancelled)
		row := WorkStatus{WorkID: item.ID, State: item.State, OriginalDue: item.OriginalDue, CurrentDue: item.CurrentDue, OverdueOriginal: open && overdue(asOf, item.OriginalDue), OverdueCurrent: open && overdue(asOf, item.CurrentDue), ScheduleChanged: len(item.ScheduleRevisions) > 0}
		if item.Wait != nil {
			row.WaitingCheckDue = overdue(asOf, item.Wait.NextCheckAt)
		}
		if item.CompletedAt != "" {
			completedAt, _ := time.Parse(time.RFC3339Nano, item.CompletedAt)
			row.CompletedLateOriginal = overdue(completedAt, item.OriginalDue)
			row.CompletedLateCurrent = overdue(completedAt, item.CurrentDue)
		}
		work = append(work, row)
	}
	releases := []ReleaseStatus{}
	for _, cycle := range in.ReleaseCycles {
		commitments := currentCommitments(cycle)
		ids := make([]string, 0, len(commitments))
		for id := range commitments {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			commitment := commitments[id]
			item := byID[id]
			deployment, evidenceAt := deploymentStatus(cycle, id, asOf)
			row := ReleaseStatus{ReleaseID: cycle.ID, WorkID: id, Commitment: commitment.Status, Readiness: item.State, Deployment: deployment, DeploymentEvidenceAt: evidenceAt, ReadyDeadlinePassed: overdue(asOf, cycle.ReadyBy), DeployDeadlinePassed: overdue(asOf, cycle.DeployBy)}
			switch {
			case commitment.Status == "excluded" || commitment.Status == "carried":
				row.Disposition = commitment.Status
			case deployment == "deployed" && item.State != Completed:
				row.Disposition = "deployed_unrecorded"
			case deployment == "deployed" && item.State == Completed:
				row.Disposition = "complete"
			case item.State != Completed:
				row.Disposition = "work_incomplete"
			case deployment == "not_deployed":
				row.Disposition = "deployment_not_done"
			default:
				row.Disposition = "deployment_unknown"
			}
			releases = append(releases, row)
		}
	}
	return work, releases
}

func deploymentStatus(cycle ReleaseCycle, workID string, asOf time.Time) (string, string) {
	status, latest := "unknown", time.Time{}
	conflict := false
	for _, evidence := range cycle.Deployments {
		if evidence.Environment != cycle.Environment || !mail.Contains(evidence.WorkIDs, workID) {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, evidence.EffectiveAt)
		if err != nil || at.After(asOf) || at.Before(latest) {
			continue
		}
		if at.Equal(latest) && status != evidence.Status {
			conflict = true
			continue
		}
		if at.After(latest) {
			status, latest, conflict = evidence.Status, at, false
		}
	}
	if conflict {
		status = "unknown"
	}
	if latest.IsZero() {
		return status, ""
	}
	return status, latest.UTC().Format(time.RFC3339Nano)
}

func ValidateReport(raw, schema []byte, in Input, observed map[string]bool) (Report, Validation, error) {
	var report Report
	validation := Validation{OperationalStatus: store.Completed, Gaps: append([]string{}, in.Gaps...), Checks: []string{}}
	compiled, err := mail.CompileSchema(schema)
	if err != nil {
		return report, validation, err
	}
	var generic any
	if err = json.Unmarshal(raw, &generic); err != nil {
		return report, validation, err
	}
	if err = compiled.Validate(generic); err != nil {
		return report, validation, fmt.Errorf("team-ops result schema: %w", err)
	}
	if err = decode(raw, &report); err != nil {
		return report, validation, err
	}
	return validateReportContract(report, in, observed, validation)
}

func validateReportContract(report Report, in Input, observed map[string]bool, validation Validation) (Report, Validation, error) {
	work, releases := ProjectStatus(in)
	if report.Version != Version || report.Scope != in.Scope || report.LedgerRevision != in.LedgerRevision || report.StateDigest != in.StateDigest || report.AsOf != in.AsOf || !sameJSON(report.WorkStatus, work) || !sameJSON(report.ReleaseStatus, releases) || !nonempty(report.Summary) || report.Risks == nil || report.Dispositions == nil || report.Limitations == nil {
		return report, validation, errors.New("team-ops report changed pinned scope, facts, schedules or release evidence")
	}
	sources := map[string]Source{}
	for _, source := range in.Sources {
		sources[source.ID] = source
		if !observed[source.ID] {
			return report, validation, errors.New("team-ops report lacks preserved source coverage")
		}
	}
	workIDs := map[string]bool{}
	for _, item := range in.WorkItems {
		workIDs[item.ID] = true
	}
	releaseIDs := map[string]bool{}
	for _, cycle := range in.ReleaseCycles {
		releaseIDs[cycle.ID] = true
	}
	incidentIDs := map[string]bool{}
	for _, incident := range in.Incidents {
		incidentIDs[incident.ID] = true
	}
	riskKeys := map[string]bool{}
	for _, risk := range report.Risks {
		if !nonempty(risk.Key) || riskKeys[risk.Key] || !oneOf(risk.Severity, "P0", "P1", "P2", "P3") || !oneOf(risk.Confidence, "high", "medium", "low") || !oneOf(risk.Kind, "overdue", "schedule_changed", "waiting_check", "release_work_incomplete", "deployment_not_done", "deployment_unknown", "deployed_unrecorded", "external_state_conflict", "slack_risk", "source_gap") || !nonempty(risk.Title) || !nonempty(risk.Impact) || !nonempty(risk.Rationale) || !nonempty(risk.RecommendedAction) || risk.WorkID != "" && !workIDs[risk.WorkID] || risk.ReleaseID != "" && !releaseIDs[risk.ReleaseID] || risk.IncidentID != "" && !incidentIDs[risk.IncidentID] || len(risk.Citations) == 0 || risk.Uncertainties == nil {
			return report, validation, errors.New("team-ops risk has invalid severity, target, reasoning or citation")
		}
		riskKeys[risk.Key] = true
		for _, citation := range risk.Citations {
			source, exists := sources[citation.SourceID]
			if !exists || len(citation.SpanIDs) == 0 {
				return report, validation, errors.New("team-ops risk cites an unknown source or no exact spans")
			}
			known := map[string]bool{}
			for _, span := range source.Spans {
				known[span.ID] = true
			}
			for _, id := range citation.SpanIDs {
				if !known[id] {
					return report, validation, errors.New("team-ops risk cites a fabricated span")
				}
			}
		}
		if risk.Confidence == "low" && len(risk.Uncertainties) == 0 {
			return report, validation, errors.New("low-confidence risk must explain its uncertainty")
		}
		if !groundedRiskKind(risk, work, releases) {
			return report, validation, errors.New("team-ops risk kind conflicts with host-computed work/deployment facts")
		}
	}
	seen := map[string]bool{}
	for _, disposition := range report.Dispositions {
		if _, exists := sources[disposition.SourceID]; !exists || seen[disposition.SourceID] || !oneOf(disposition.Outcome, "risk", "context", "no_action", "uncertain") || !nonempty(disposition.Reason) || disposition.RiskKeys == nil {
			return report, validation, errors.New("team-ops source disposition is unknown, duplicated or incomplete")
		}
		seen[disposition.SourceID] = true
		for _, key := range disposition.RiskKeys {
			if !riskKeys[key] {
				return report, validation, errors.New("source disposition references an unknown risk")
			}
		}
	}
	if len(seen) != len(sources) {
		return report, validation, errors.New("team-ops report omitted a source disposition")
	}
	validation.Checks = []string{"result_schema", "pinned_scope_revision", "host_work_schedule_release_facts", "complete_source_dispositions", "host_span_citations"}
	validation.Gaps = append(validation.Gaps, report.Limitations...)
	if len(validation.Gaps) > 0 {
		validation.OperationalStatus = store.Partial
	}
	return report, validation, nil
}

func groundedRiskKind(risk Risk, work []WorkStatus, releases []ReleaseStatus) bool {
	switch risk.Kind {
	case "overdue", "schedule_changed", "waiting_check":
		for _, row := range work {
			if row.WorkID == risk.WorkID {
				return risk.Kind == "overdue" && row.OverdueCurrent || risk.Kind == "schedule_changed" && row.ScheduleChanged || risk.Kind == "waiting_check" && row.WaitingCheckDue
			}
		}
		return false
	case "release_work_incomplete", "deployment_not_done", "deployment_unknown", "deployed_unrecorded":
		for _, row := range releases {
			if row.ReleaseID != risk.ReleaseID || risk.WorkID != "" && row.WorkID != risk.WorkID || row.Commitment != "included" {
				continue
			}
			if risk.Kind == "release_work_incomplete" && row.Readiness != Completed || risk.Kind == "deployment_not_done" && row.Deployment == "not_deployed" || risk.Kind == "deployment_unknown" && row.Deployment == "unknown" || risk.Kind == "deployed_unrecorded" && row.Disposition == "deployed_unrecorded" {
				return true
			}
		}
		return false
	}
	return true
}

func RenderReport(report Report, validation Validation, in Input) []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "# Team operations — %s / %s\n\n%s\n\nAs of %s; ledger revision %d. Saved-input shadow analysis.\n\n", in.Scope.Team, in.Scope.Project, report.Summary, in.AsOf, in.LedgerRevision)
	out.WriteString("## Work and schedules\n\n| Work | State | Original due | Current due | Current overdue | Schedule changed |\n|---|---|---|---|---|---|\n")
	for _, row := range report.WorkStatus {
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %t | %t |\n", row.WorkID, row.State, row.OriginalDue, row.CurrentDue, row.OverdueCurrent, row.ScheduleChanged)
	}
	out.WriteString("\n## Release commitments and evidence\n\n| Release | Work | Commitment | Work state | Deployment evidence | Outcome |\n|---|---|---|---|---|---|\n")
	for _, row := range report.ReleaseStatus {
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s |\n", row.ReleaseID, row.WorkID, row.Commitment, row.Readiness, row.Deployment, row.Disposition)
	}
	out.WriteString("\n## Reasoned risks and next actions\n\n")
	for _, risk := range report.Risks {
		fmt.Fprintf(&out, "### %s — %s (%s confidence)\n\n%s\n\nImpact: %s\n\nNext: %s\n\n", risk.Severity, risk.Title, risk.Confidence, risk.Rationale, risk.Impact, risk.RecommendedAction)
		for _, citation := range risk.Citations {
			fmt.Fprintf(&out, "Evidence: %s; spans %v.\n\n", citation.SourceID, citation.SpanIDs)
		}
		for _, uncertainty := range risk.Uncertainties {
			fmt.Fprintf(&out, "Uncertainty: %s\n\n", uncertainty)
		}
	}
	out.WriteString("## Validation and limits\n\nThe host checked pinned facts, full source disposition coverage and exact span IDs. Semantic judgment remains independently reviewable. Work state is changed only by explicit owner commands. Notification plans are local shadow records; no external message was sent. Saved capture coverage does not establish continuous live monitoring.\n\n")
	for _, gap := range validation.Gaps {
		fmt.Fprintf(&out, "- %s\n", gap)
	}
	return out.Bytes()
}

func riskFingerprint(risk Risk) string {
	refs := append([]Citation{}, risk.Citations...)
	for i := range refs {
		refs[i].SpanIDs = append([]string{}, refs[i].SpanIDs...)
		sort.Strings(refs[i].SpanIDs)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].SourceID < refs[j].SourceID })
	return files.Digest(mustJSON(struct {
		Kind, Work, Release, Severity string
		Citations                     []Citation
	}{risk.Kind, risk.WorkID, risk.ReleaseID, risk.Severity, refs}))
}
