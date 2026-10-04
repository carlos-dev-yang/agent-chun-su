package ops

import (
	"errors"
	"sort"
	"strconv"
	"time"

	"chunsu/internal/files"
)

func incidentKey(risk Risk) string {
	key := risk.Kind + "|" + risk.WorkID + "|" + risk.ReleaseID
	if risk.WorkID == "" && risk.ReleaseID == "" {
		ids := []string{}
		for _, citation := range risk.Citations {
			ids = append(ids, citation.SourceID)
		}
		sort.Strings(ids)
		key += "|" + string(mustJSON(ids))
	}
	return key
}

func applyAssessment(state *State, assessment Assessment, at string) error {
	if !files.ValidID(assessment.JobID) || !files.ValidID(assessment.Artifact.ID) || !files.ValidDigest(assessment.Artifact.Digest) || !files.ValidDigest(assessment.InputDigest) || !files.ValidDigest(assessment.BundleDigest) || assessment.LedgerRevision != state.Revision || assessment.Report.LedgerRevision != state.Revision || assessment.Report.Scope != state.Scope {
		return errors.New("assessment is stale or lacks pinned job, input, artifact and Skill provenance")
	}
	for _, previous := range state.Assessments {
		if previous.JobID == assessment.JobID {
			return errors.New("this assessment job was already reconciled; inspect its receipt")
		}
	}
	seen := map[string]bool{}
	for _, risk := range assessment.Report.Risks {
		key := incidentKey(risk)
		id := files.Digest([]byte(state.Scope.Key() + "|" + key))
		if risk.IncidentID != "" {
			prior, exists := state.Incidents[risk.IncidentID]
			if !exists || prior.Risk.Kind != risk.Kind || prior.Risk.WorkID != risk.WorkID || prior.Risk.ReleaseID != risk.ReleaseID {
				return errors.New("incident linking proposal conflicts with the existing incident target")
			}
			id, key = prior.ID, prior.Key
		}
		if seen[id] {
			return errors.New("assessment contains duplicate risks for the same effective incident identity")
		}
		seen[id] = true
		incident, exists := state.Incidents[id]
		fingerprint := riskFingerprint(risk)
		if exists && incident.Fingerprint == fingerprint {
			// Preserve corrected judgments while deduplicating delivery on exact
			// evidence/severity. Wording alone does not create another call plan.
			if !sameJSON(incident.Risk, risk) {
				incident.Revision++
				incident.Risk, incident.AssessmentJobID, incident.AssessmentArtifact, incident.UpdatedAt = risk, assessment.JobID, assessment.Artifact, at
				state.Incidents[id] = incident
			}
			continue
		}
		previousSeverity := incident.Risk.Severity
		if !exists {
			incident = Incident{ID: id, Key: key, State: "open", CreatedAt: at, ResolutionEvidence: []Evidence{}}
		}
		if incident.State == "resolved" {
			incident.State = "open"
			incident.AcknowledgedAt = ""
			incident.AcknowledgedBy = ""
		}
		incident.Revision++
		incident.Risk, incident.Fingerprint, incident.AssessmentJobID, incident.AssessmentArtifact, incident.UpdatedAt = risk, fingerprint, assessment.JobID, assessment.Artifact, at
		state.Incidents[id] = incident
		planNotification(state, incident, previousSeverity, at)
	}
	state.Assessments = append(state.Assessments, assessment)
	return nil
}

func planNotification(state *State, incident Incident, previousSeverity, at string, suffix ...string) {
	rule := state.Policy.Rules[incident.Risk.Severity]
	key := incident.ID + "|" + strconv.FormatInt(incident.Revision, 10) + "|" + string(mustJSON(rule.Recipients))
	if len(suffix) == 1 {
		key += "|" + suffix[0]
	}
	for _, plan := range state.Notifications {
		if plan.Key == key {
			return
		}
	}
	plan := Notification{ID: files.Digest([]byte(key)), IncidentID: incident.ID, IncidentRevision: incident.Revision, Key: key, Severity: incident.Risk.Severity, Mode: rule.Mode, Recipients: append([]string{}, rule.Recipients...), Status: "shadow_planned", Reason: "local preview only; no sender is installed", CreatedAt: at, Policy: state.Policy}
	now, _ := time.Parse(time.RFC3339Nano, at)
	escalated := previousSeverity != "" && severityRank(incident.Risk.Severity) < severityRank(previousSeverity)
	switch {
	case rule.Mode == "none":
		plan.Status = "suppressed"
		plan.Reason = "policy keeps this severity in the local report"
	case len(rule.Recipients) == 0:
		plan.Status = "shadow_blocked"
		plan.Reason = "recipient mapping is unset; no delivery was attempted"
	case notificationSnoozed(incident, state.Policy, now) && !escalated:
		plan.Status = "suppressed"
		plan.Reason = "owner snooze remains active until " + incident.SnoozedUntil
	}
	if plan.Status == "shadow_planned" && rule.CooldownSeconds > 0 && !escalated {
		for index := len(state.Notifications) - 1; index >= 0; index-- {
			previous := state.Notifications[index]
			if previous.IncidentID != incident.ID || previous.Status != "shadow_planned" {
				continue
			}
			previousAt, _ := time.Parse(time.RFC3339Nano, previous.CreatedAt)
			if now.Sub(previousAt) < time.Duration(rule.CooldownSeconds)*time.Second {
				plan.Status = "suppressed"
				plan.Reason = "configured incident cooldown"
			}
			break
		}
	}
	if plan.Status == "shadow_planned" && rule.AcknowledgmentSeconds > 0 {
		plan.AcknowledgmentDue = now.Add(time.Duration(rule.AcknowledgmentSeconds) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	state.Notifications = append(state.Notifications, plan)
}

// ReviewNotifications advances eligible shadow previews after a bounded
// suppression expires or owner policy changes. It never retries a real send.
func ReviewNotifications(state *State, at string) {
	now, _ := time.Parse(time.RFC3339Nano, at)
	ids := make([]string, 0, len(state.Incidents))
	for id := range state.Incidents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		incident := state.Incidents[id]
		if incident.State == "resolved" {
			continue
		}
		var latest *Notification
		for index := len(state.Notifications) - 1; index >= 0; index-- {
			if state.Notifications[index].IncidentID == id {
				copy := state.Notifications[index]
				latest = &copy
				break
			}
		}
		if latest != nil && latest.Status == "shadow_planned" && sameJSON(latest.Policy, state.Policy) && !notificationSnoozed(incident, state.Policy, now) {
			continue
		}
		preview := *state
		preview.Notifications = append([]Notification{}, state.Notifications...)
		planNotification(&preview, incident, "", at, "review:"+strconv.FormatInt(state.Revision+1, 10))
		if len(preview.Notifications) == len(state.Notifications) {
			continue
		}
		candidate := preview.Notifications[len(preview.Notifications)-1]
		if latest != nil && candidate.Status == latest.Status && candidate.Reason == latest.Reason && sameJSON(candidate.Policy, latest.Policy) {
			continue
		}
		state.Notifications = append(state.Notifications, candidate)
	}
}

func notificationSnoozed(incident Incident, policy Policy, now time.Time) bool {
	if incident.SnoozedUntil == "" || (incident.Risk.Severity == "P0" && policy.P0BypassesSnooze) {
		return false
	}
	until, _ := time.Parse(time.RFC3339Nano, incident.SnoozedUntil)
	return now.Before(until)
}

func severityRank(severity string) int {
	for index, value := range []string{"P0", "P1", "P2", "P3"} {
		if value == severity {
			return index
		}
	}
	return len(DefaultPolicy().Rules)
}

func applyIncidentChange(state *State, command Command, at string) error {
	incident, exists := state.Incidents[command.EntityID]
	if !exists {
		return errors.New("incident does not exist")
	}
	in, err := payload[IncidentChange](command)
	if err != nil {
		return err
	}
	if !oneOf(in.State, "acknowledged", "responding", "resolved", "open", "snoozed") {
		return errors.New("incident action must acknowledge, respond, resolve, reopen or snooze")
	}
	if in.State == "open" && incident.State != "resolved" {
		return errors.New("only a resolved incident can be explicitly reopened")
	}
	if incident.State == "resolved" && in.State != "open" {
		return errors.New("resolved incident must be explicitly reopened before another action")
	}
	if in.State == "snoozed" {
		if !timestamp(in.SnoozedUntil) {
			return errors.New("snooze needs a bounded RFC3339 expiry")
		}
		until, _ := time.Parse(time.RFC3339Nano, in.SnoozedUntil)
		now, _ := time.Parse(time.RFC3339Nano, at)
		if !until.After(now) {
			return errors.New("snooze expiry must be in the future")
		}
		incident.SnoozedUntil = in.SnoozedUntil
	} else {
		if in.SnoozedUntil != "" {
			return errors.New("snooze expiry belongs only to snooze action")
		}
		if err = validateEvidence(*state, in.Evidence, in.State == "resolved"); err != nil {
			return err
		}
		incident.State = in.State
		if in.State == "acknowledged" || in.State == "responding" {
			incident.AcknowledgedBy = command.Actor
			incident.AcknowledgedAt = at
		}
		if in.State == "resolved" {
			incident.ResolutionEvidence = append(incident.ResolutionEvidence, in.Evidence...)
		}
		if in.State == "open" {
			incident.AcknowledgedAt = ""
			incident.AcknowledgedBy = ""
			incident.SnoozedUntil = ""
		}
	}
	incident.Revision++
	incident.UpdatedAt = at
	state.Incidents[incident.ID] = incident
	if in.State == "open" {
		planNotification(state, incident, "", at)
	}
	return nil
}
