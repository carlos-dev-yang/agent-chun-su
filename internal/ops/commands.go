package ops

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"chunsu/internal/config"
)

func payload[T any](command Command) (T, error) {
	var value T
	err := decode(command.Data, &value)
	return value, err
}

func apply(state *State, command Command, at string, limits config.Limits) error {
	if state.Revision == 0 && command.Action != "scope.init" {
		return errors.New("OPS scope is not initialized")
	}
	switch command.Action {
	case "scope.init":
		if state.Revision != 0 {
			return errors.New("OPS scope already exists")
		}
		settings, err := payload[Settings](command)
		if err != nil {
			return err
		}
		if settings.Timezone == "" || settings.Timezone == "Local" {
			return errors.New("OPS scope needs an explicit IANA timezone")
		}
		if _, err = time.LoadLocation(settings.Timezone); err != nil {
			return errors.New("OPS timezone is invalid")
		}
		state.Settings = settings
	case "work.create":
		in, err := payload[WorkCreate](command)
		if err != nil {
			return err
		}
		if !identifier(command.EntityID) || !nonempty(in.Title) || !nonempty(in.Owner) || !nonempty(in.CompletionCriteria) || in.Due != "" && !timestamp(in.Due) {
			return errors.New("work creation needs ID, title, owner, completion criteria and an optional RFC3339 due time")
		}
		if _, exists := state.WorkItems[command.EntityID]; exists {
			return errors.New("work item already exists")
		}
		if err = validateEvidence(*state, in.Evidence, false); err != nil {
			return err
		}
		state.WorkItems[command.EntityID] = WorkItem{ID: command.EntityID, Title: in.Title, Owner: in.Owner, CompletionCriteria: in.CompletionCriteria, State: Ready, OriginalDue: in.Due, CurrentDue: in.Due, ScheduleRevisions: []ScheduleRevision{}, Evidence: append([]Evidence{}, in.Evidence...), CreatedAt: at, UpdatedAt: at}
	case "work.transition":
		item, exists := state.WorkItems[command.EntityID]
		if !exists {
			return errors.New("work item does not exist")
		}
		in, err := payload[WorkTransition](command)
		if err != nil {
			return err
		}
		if !oneOf(in.State, Ready, InProgress, Waiting, Completed, Cancelled) || in.State == item.State {
			return errors.New("invalid or unchanged work state")
		}
		if oneOf(item.State, Completed, Cancelled) && !oneOf(in.State, Ready, InProgress) {
			return errors.New("closed work can only be explicitly reopened to ready or inprogress")
		}
		if in.State == Waiting && (in.Wait == nil || !nonempty(in.Wait.Reason) || !nonempty(in.Wait.Responsible) || !timestamp(in.Wait.NextCheckAt)) {
			return errors.New("waiting needs a reason, responsible person and RFC3339 next check")
		}
		if in.State != Waiting && in.Wait != nil {
			return errors.New("wait details belong only to waiting work")
		}
		if err = validateEvidence(*state, in.Evidence, in.State == Completed); err != nil {
			return err
		}
		item.State, item.Wait, item.UpdatedAt = in.State, in.Wait, at
		item.Evidence = append(item.Evidence, in.Evidence...)
		if in.State == Completed {
			item.CompletedAt = at
		} else {
			item.CompletedAt = ""
		}
		state.WorkItems[item.ID] = item
	case "work.reschedule":
		item, exists := state.WorkItems[command.EntityID]
		if !exists {
			return errors.New("work item does not exist")
		}
		in, err := payload[Reschedule](command)
		if err != nil {
			return err
		}
		if !timestamp(in.Due) || in.Due == item.CurrentDue {
			return errors.New("reschedule needs a changed RFC3339 due time")
		}
		item.ScheduleRevisions = append(item.ScheduleRevisions, ScheduleRevision{PreviousDue: item.CurrentDue, Due: in.Due, Reason: command.Reason, Actor: command.Actor, At: at, LedgerRevision: state.Revision + 1})
		item.CurrentDue, item.UpdatedAt = in.Due, at
		state.WorkItems[item.ID] = item
	case "release.create":
		in, err := payload[ReleaseCreate](command)
		if err != nil {
			return err
		}
		if !identifier(command.EntityID) || !nonempty(in.Service) || !nonempty(in.Environment) || !timestamp(in.ReadyBy) || !timestamp(in.DeployBy) {
			return errors.New("release needs ID, service, environment and RFC3339 readiness/deployment deadlines")
		}
		ready, _ := time.Parse(time.RFC3339Nano, in.ReadyBy)
		deploy, _ := time.Parse(time.RFC3339Nano, in.DeployBy)
		if ready.After(deploy) {
			return errors.New("release readiness deadline is after deployment deadline")
		}
		if _, exists := state.ReleaseCycles[command.EntityID]; exists {
			return errors.New("release cycle already exists")
		}
		state.ReleaseCycles[command.EntityID] = ReleaseCycle{ID: command.EntityID, Service: in.Service, Environment: in.Environment, ReadyBy: in.ReadyBy, DeployBy: in.DeployBy, Commitments: []Commitment{}, Deployments: []Deployment{}}
	case "release.target":
		cycle, exists := state.ReleaseCycles[command.EntityID]
		if !exists {
			return errors.New("release cycle does not exist")
		}
		in, err := payload[TargetChange](command)
		if err != nil {
			return err
		}
		if _, exists = state.WorkItems[in.WorkID]; !exists || !oneOf(in.Status, "included", "excluded", "carried") {
			return errors.New("release target needs an existing work item and included, excluded or carried status")
		}
		current, present := currentCommitments(cycle)[in.WorkID]
		if (!present || current.Status != "included") && in.Status != "included" || present && current.Status == in.Status {
			return errors.New("release target change is inconsistent with the current commitment")
		}
		if in.Status != "carried" && in.CarriedTo != "" {
			return errors.New("carried_to belongs only to a carried target")
		}
		if in.Status == "carried" {
			next, present := state.ReleaseCycles[in.CarriedTo]
			if !present || next.ID == cycle.ID || next.Service != cycle.Service || next.Environment != cycle.Environment {
				return errors.New("carry requires a different existing cycle for the same service and environment")
			}
			currentDeadline, _ := time.Parse(time.RFC3339Nano, cycle.DeployBy)
			nextDeadline, _ := time.Parse(time.RFC3339Nano, next.DeployBy)
			if !nextDeadline.After(currentDeadline) {
				return errors.New("carry requires a later deployment cycle")
			}
			if target, exists := currentCommitments(next)[in.WorkID]; exists && target.Status == "included" {
				return errors.New("target is already included in the destination cycle")
			}
			next.CommitmentRevision++
			next.Commitments = append(next.Commitments, Commitment{WorkID: in.WorkID, Status: "included", Reason: command.Reason, Actor: command.Actor, At: at, Revision: next.CommitmentRevision})
			state.ReleaseCycles[next.ID] = next
		}
		cycle.CommitmentRevision++
		cycle.Commitments = append(cycle.Commitments, Commitment{WorkID: in.WorkID, Status: in.Status, CarriedTo: in.CarriedTo, Reason: command.Reason, Actor: command.Actor, At: at, Revision: cycle.CommitmentRevision})
		state.ReleaseCycles[cycle.ID] = cycle
	case "release.deployment":
		cycle, exists := state.ReleaseCycles[command.EntityID]
		if !exists {
			return errors.New("release cycle does not exist")
		}
		in, err := payload[Deployment](command)
		if err != nil {
			return err
		}
		if !oneOf(in.Status, "deployed", "not_deployed", "unknown") || in.Environment != cycle.Environment || !timestamp(in.EffectiveAt) || len(in.WorkIDs) == 0 {
			return errors.New("deployment evidence needs status, exact release environment, item IDs and effective time")
		}
		if in.Actor != "" || in.RecordedAt != "" {
			return errors.New("deployment actor and recorded_at are host-owned")
		}
		seen := map[string]bool{}
		for _, id := range in.WorkIDs {
			if _, exists := state.WorkItems[id]; !exists || seen[id] {
				return errors.New("deployment evidence references unknown or duplicate work")
			}
			seen[id] = true
		}
		if err = validateEvidence(*state, in.Evidence, in.Status != "unknown"); err != nil {
			return err
		}
		in.Actor, in.RecordedAt = command.Actor, at
		cycle.Deployments = append(cycle.Deployments, in)
		state.ReleaseCycles[cycle.ID] = cycle
	case "observation.import":
		observation, err := payload[Observation](command)
		if err != nil {
			return err
		}
		if !identifier(command.EntityID) || observation.ID != command.EntityID || observation.Scope != state.Scope {
			return errors.New("observation identity or scope mismatch")
		}
		normalized, normalizeErr := NormalizeObservation(observation.ID, observation.Scope, observation.Provider, observation.Raw, limits)
		if normalizeErr != nil {
			return normalizeErr
		}
		if !sameJSON(normalized, observation) {
			return errors.New("saved observation projection, synthetic declaration or provenance differs from its exact raw input")
		}
		if _, exists := state.Observations[observation.ID]; exists {
			return errors.New("observation ID already exists; new versions need another preserved import")
		}
		for _, previous := range state.Observations {
			if previous.InputDigest == observation.InputDigest && previous.Provider == observation.Provider {
				return errors.New("this exact saved observation was already imported")
			}
			for _, source := range observation.Sources {
				for _, old := range previous.Sources {
					if source.ID == old.ID && !sameSourceVersion(source, old) {
						return errors.New("same source identity/version has conflicting evidence; preserve a new version")
					}
				}
			}
		}
		state.Observations[observation.ID] = observation
	case "policy.set":
		policy, err := payload[Policy](command)
		if err != nil {
			return err
		}
		if err = policy.Validate(); err != nil {
			return err
		}
		state.Policy = policy
	case "assessment.apply":
		assessment, err := payload[Assessment](command)
		if err != nil {
			return err
		}
		return applyAssessment(state, assessment, at)
	case "incident.change":
		return applyIncidentChange(state, command, at)
	case "notification.review":
		var empty struct{}
		if err := decode(command.Data, &empty); err != nil {
			return err
		}
		ReviewNotifications(state, at)
	default:
		return fmt.Errorf("unsupported OPS action %q", command.Action)
	}
	return nil
}

func sameJSON(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func currentCommitments(cycle ReleaseCycle) map[string]Commitment {
	out := map[string]Commitment{}
	for _, commitment := range cycle.Commitments {
		out[commitment.WorkID] = commitment
	}
	return out
}

func validateEvidence(state State, evidence []Evidence, required bool) error {
	if required && len(evidence) == 0 {
		return errors.New("this state change needs completion/deployment/resolution evidence or an explicit owner confirmation")
	}
	for _, item := range evidence {
		if !nonempty(item.Statement) {
			return errors.New("evidence needs a verifiable statement or owner confirmation")
		}
		if item.ObservationID == "" {
			if item.SourceID != "" || len(item.SpanIDs) != 0 {
				return errors.New("source/span evidence needs an observation ID")
			}
			continue
		}
		observation, exists := state.Observations[item.ObservationID]
		if !exists {
			return errors.New("evidence observation does not exist in this scope")
		}
		if item.SourceID == "" {
			return errors.New("observation evidence needs an exact source ID")
		}
		found := false
		for _, source := range observation.Sources {
			if source.ID != item.SourceID {
				continue
			}
			found = true
			known := map[string]bool{}
			for _, span := range source.Spans {
				known[span.ID] = true
			}
			for _, id := range item.SpanIDs {
				if !known[id] {
					return errors.New("evidence span is absent from the selected observation")
				}
			}
		}
		if !found {
			return errors.New("evidence source is absent from the selected observation")
		}
	}
	return nil
}

func (p Policy) Validate() error {
	if p.Version != Version || len(p.Rules) != len(DefaultPolicy().Rules) || !validDuration(p.SourceFreshnessSeconds) {
		return errors.New("OPS policy needs version 1, P0..P3 rules and bounded explicit durations")
	}
	for severity := range DefaultPolicy().Rules {
		rule, exists := p.Rules[severity]
		if !exists || !oneOf(rule.Mode, "immediate", "digest", "none") || !validDuration(rule.CooldownSeconds) || !validDuration(rule.AcknowledgmentSeconds) {
			return errors.New("OPS severity policy has invalid mode or duration")
		}
		seen := map[string]bool{}
		for _, recipient := range rule.Recipients {
			if !nonempty(recipient) || seen[recipient] {
				return errors.New("OPS recipients must be nonempty and unique owner-managed labels")
			}
			seen[recipient] = true
		}
	}
	return nil
}
