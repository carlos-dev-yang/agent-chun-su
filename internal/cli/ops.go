package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/ops"
	"chunsu/internal/runner"
	"chunsu/internal/workgroup"
	"github.com/spf13/cobra"
)

type opsFlags struct {
	Team, Project, Actor, Reason, OperationID string
	Revision                                  int64
}

func (f opsFlags) scope() ops.Scope { return ops.Scope{Team: f.Team, Project: f.Project} }

func (f opsFlags) command(action, entity string, data any) ops.Command {
	operation := f.OperationID
	if operation == "" {
		operation = files.ID()
	}
	payload, _ := json.Marshal(data)
	return ops.Command{Version: ops.Version, Scope: f.scope(), OperationID: operation, ExpectedRevision: f.Revision, Actor: f.Actor, Reason: f.Reason, Action: action, EntityID: entity, Data: payload}
}

func ownerEvidence(statements []string) []ops.Evidence {
	out := []ops.Evidence{}
	for _, statement := range statements {
		out = append(out, ops.Evidence{Statement: statement})
	}
	return out
}

func (o *options) ops() *cobra.Command {
	cmd := &cobra.Command{Use: "ops", Short: "Operate a local owner-controlled team work/release ledger and saved-input shadow reports"}
	var flags opsFlags
	cmd.PersistentFlags().StringVar(&flags.Team, "team", "", "Explicit team namespace; not team-user authorization")
	cmd.PersistentFlags().StringVar(&flags.Project, "project", "", "Explicit project namespace")
	cmd.PersistentFlags().StringVar(&flags.Actor, "actor", "", "Owner label for the audit trail")
	cmd.PersistentFlags().StringVar(&flags.Reason, "reason", "", "Reason for a confirmed local change")
	cmd.PersistentFlags().StringVar(&flags.OperationID, "operation-id", "", "Stable operation ID for a retry after an uncertain result")
	cmd.PersistentFlags().Int64Var(&flags.Revision, "revision", -1, "Expected current scope revision; required for every business mutation")
	_ = cmd.MarkPersistentFlagRequired("team")
	_ = cmd.MarkPersistentFlagRequired("project")
	mutate := func(command *cobra.Command, action, entity string, value any) error {
		return o.opsApply(command, flags.command(action, entity, value), "ops_command")
	}
	read := func(command *cobra.Command) (ops.Service, func(), error) {
		if err := flags.scope().Validate(); err != nil {
			return ops.Service{}, nil, err
		}
		s, c, closeStore, err := o.open(command.Context(), false)
		return ops.Service{Store: s, Config: c}, closeStore, err
	}
	scope := &cobra.Command{Use: "scope", Short: "Initialize a private owner-operated team/project namespace"}
	var timezone string
	var synthetic bool
	initialize := &cobra.Command{Use: "init", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "scope.init", "", ops.Settings{Timezone: timezone, Synthetic: synthetic})
	}}
	initialize.Flags().StringVar(&timezone, "timezone", "", "Explicit IANA timezone")
	initialize.Flags().BoolVar(&synthetic, "synthetic", false, "Declare this scope contains public synthetic fixtures; never relabel real saved evidence")
	_ = initialize.MarkFlagRequired("timezone")
	scope.AddCommand(initialize)

	work := &cobra.Command{Use: "work", Short: "Confirm work state independently from external Jira status"}
	var title, owner, criteria, due string
	var createEvidence []string
	create := &cobra.Command{Use: "create WORK_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "work.create", args[0], ops.WorkCreate{Title: title, Owner: owner, CompletionCriteria: criteria, Due: due, Evidence: ownerEvidence(createEvidence)})
	}}
	create.Flags().StringVar(&title, "title", "", "Work title")
	create.Flags().StringVar(&owner, "owner", "", "Responsible person label")
	create.Flags().StringVar(&criteria, "completion-criteria", "", "What evidence makes the work complete")
	create.Flags().StringVar(&due, "due", "", "Optional RFC3339 deadline, with an offset")
	create.Flags().StringArrayVar(&createEvidence, "evidence", nil, "Owner confirmation or evidence statement (repeatable)")
	var waitReason, responsible, nextCheck string
	var transitionEvidence []string
	transition := &cobra.Command{Use: "transition WORK_ID ready|inprogress|waiting|completed|cancelled", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		value := ops.WorkTransition{State: args[1], Evidence: ownerEvidence(transitionEvidence)}
		if waitReason != "" || responsible != "" || nextCheck != "" {
			value.Wait = &ops.WaitDetail{Reason: waitReason, Responsible: responsible, NextCheckAt: nextCheck}
		}
		return mutate(command, "work.transition", args[0], value)
	}}
	transition.Flags().StringVar(&waitReason, "waiting-reason", "", "Blocker or wait reason")
	transition.Flags().StringVar(&responsible, "responsible", "", "Person responsible for resolving the wait")
	transition.Flags().StringVar(&nextCheck, "next-check", "", "RFC3339 next check time")
	transition.Flags().StringArrayVar(&transitionEvidence, "evidence", nil, "Completion/transition evidence or owner confirmation (repeatable)")
	var newDue string
	reschedule := &cobra.Command{Use: "reschedule WORK_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "work.reschedule", args[0], ops.Reschedule{Due: newDue})
	}}
	reschedule.Flags().StringVar(&newDue, "due", "", "New approved RFC3339 deadline; original promise remains in history")
	work.AddCommand(create, transition, reschedule)

	release := &cobra.Command{Use: "release", Short: "Record release commitments and actual deployment evidence as separate facts"}
	var service, environment, readyBy, deployBy string
	newRelease := &cobra.Command{Use: "create CYCLE_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "release.create", args[0], ops.ReleaseCreate{Service: service, Environment: environment, ReadyBy: readyBy, DeployBy: deployBy})
	}}
	newRelease.Flags().StringVar(&service, "service", "", "Target service label")
	newRelease.Flags().StringVar(&environment, "environment", "", "Exact target environment")
	newRelease.Flags().StringVar(&readyBy, "ready-by", "", "RFC3339 work readiness deadline")
	newRelease.Flags().StringVar(&deployBy, "deploy-by", "", "RFC3339 deployment deadline")
	var carriedTo string
	target := &cobra.Command{Use: "target CYCLE_ID WORK_ID included|excluded|carried", Args: cobra.ExactArgs(3), RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "release.target", args[0], ops.TargetChange{WorkID: args[1], Status: args[2], CarriedTo: carriedTo})
	}}
	target.Flags().StringVar(&carriedTo, "to-cycle", "", "Existing later cycle for approved carry-forward")
	var deploymentEnvironment, effectiveAt string
	var deploymentWork, deploymentEvidence []string
	deployment := &cobra.Command{Use: "deployment CYCLE_ID deployed|not_deployed|unknown", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "release.deployment", args[0], ops.Deployment{Status: args[1], Environment: deploymentEnvironment, WorkIDs: deploymentWork, EffectiveAt: effectiveAt, Evidence: ownerEvidence(deploymentEvidence)})
	}}
	deployment.Flags().StringVar(&deploymentEnvironment, "environment", "", "Exact release environment")
	deployment.Flags().StringVar(&effectiveAt, "effective-at", "", "RFC3339 time the evidence establishes")
	deployment.Flags().StringArrayVar(&deploymentWork, "work", nil, "Exact work item included in the evidence (repeatable)")
	deployment.Flags().StringArrayVar(&deploymentEvidence, "evidence", nil, "Deployment evidence or owner confirmation (repeatable)")
	release.AddCommand(newRelease, target, deployment)

	observation := &cobra.Command{Use: "observation", Short: "Import saved mail/Jira/Slack evidence, preserving exact raw bytes and gaps"}
	var format string
	importObservation := &cobra.Command{Use: "import OBSERVATION_ID INPUT_JSON", Args: cobra.ExactArgs(2), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		data, err := readExternal(args[1], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		value, err := ops.NormalizeObservation(args[0], flags.scope(), format, data, c.Limits)
		if err != nil {
			return err
		}
		return mutate(command, "observation.import", args[0], value)
	}}
	importObservation.Flags().StringVar(&format, "format", "", "Saved envelope format: mail, jira or slack")
	observation.AddCommand(importObservation)
	policy := &cobra.Command{Use: "policy", Short: "Configure local P0..P3 preview routing, cooldown and response timing"}
	policy.AddCommand(&cobra.Command{Use: "set POLICY_JSON", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		data, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		var value ops.Policy
		if err = mail.Decode(data, &value); err != nil {
			return err
		}
		return mutate(command, "policy.set", "", value)
	}})
	incident := &cobra.Command{Use: "incident", Short: "Record acknowledgment, response, resolution, reopen or bounded snooze"}
	for _, action := range []struct{ name, state string }{{"ack", "acknowledged"}, {"respond", "responding"}, {"resolve", "resolved"}, {"reopen", "open"}, {"snooze", "snoozed"}} {
		action := action
		var statements []string
		var until string
		change := &cobra.Command{Use: action.name + " INCIDENT_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
			return mutate(command, "incident.change", args[0], ops.IncidentChange{State: action.state, SnoozedUntil: until, Evidence: ownerEvidence(statements)})
		}}
		change.Flags().StringArrayVar(&statements, "evidence", nil, "Resolution/action evidence or owner confirmation (repeatable)")
		change.Flags().StringVar(&until, "until", "", "RFC3339 snooze expiry; snooze only")
		incident.AddCommand(change)
	}
	show := &cobra.Command{Use: "show", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		state, _, err := s.Load(command.Context(), flags.scope())
		if err != nil {
			return err
		}
		return output(command, state)
	}}
	history := &cobra.Command{Use: "history", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		_, entries, err := s.Load(command.Context(), flags.scope())
		if err != nil {
			return err
		}
		return output(command, entries)
	}}
	var statusAt string
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		in, err := s.BuildInput(command.Context(), flags.scope(), opsAsOf(statusAt))
		if err != nil {
			return err
		}
		work, releases := ops.ProjectStatus(in)
		return output(command, map[string]any{"scope": in.Scope, "revision": in.LedgerRevision, "work": work, "releases": releases, "incidents": in.Incidents, "gaps": in.Gaps, "monitoring": "saved_input_only", "delivery": "shadow_only"})
	}}
	status.Flags().StringVar(&statusAt, "as-of", "", "RFC3339 report time; defaults to now")
	var snapshotAt string
	snapshot := &cobra.Command{Use: "snapshot", Args: cobra.NoArgs, Short: "Build the verified current saved report input", RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		in, err := s.BuildInput(command.Context(), flags.scope(), opsAsOf(snapshotAt))
		if err != nil {
			return err
		}
		return output(command, in)
	}}
	snapshot.Flags().StringVar(&snapshotAt, "as-of", "", "RFC3339 report time; defaults to now")
	sources := &cobra.Command{Use: "sources OBSERVATION_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		state, _, err := s.Load(command.Context(), flags.scope())
		if err != nil {
			return err
		}
		value, exists := state.Observations[args[0]]
		if !exists {
			return errors.New("observation does not exist in this scope")
		}
		return output(command, map[string]any{"id": value.ID, "provider": value.Provider, "synthetic": value.Synthetic, "coverage": value.Coverage, "gaps": value.Gaps, "captured_from": value.CapturedFrom, "captured_through": value.CapturedThrough, "sources": value.Sources})
	}}
	readReceipt := &cobra.Command{Use: "receipt OPERATION_ID", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		_, entries, err := s.Load(command.Context(), flags.scope())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Event.Command.OperationID == args[0] {
				return output(command, entry)
			}
		}
		return errors.New("operation is not committed in this scope")
	}}
	outbox := &cobra.Command{Use: "outbox", Short: "Inspect persistent local plans; no sender or live delivery is enabled"}
	outbox.AddCommand(&cobra.Command{Use: "review", Args: cobra.NoArgs, Short: "Replan eligible shadow previews after suppression expires or policy changes", RunE: func(command *cobra.Command, args []string) error {
		return mutate(command, "notification.review", "", struct{}{})
	}})
	outbox.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		s, closeStore, err := read(command)
		if err != nil {
			return err
		}
		defer closeStore()
		state, _, err := s.Load(command.Context(), flags.scope())
		if err != nil {
			return err
		}
		return output(command, map[string]any{"mode": "shadow_only", "notifications": state.Notifications})
	}})
	var objective, reportAt, candidate string
	var checkpoint bool
	report := &cobra.Command{Use: "report", Args: cobra.NoArgs, Short: "Queue actual AI risk synthesis through FLOW for synthetic saved evidence", RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		if err = flags.scope().Validate(); err != nil {
			return err
		}
		input, _ := json.Marshal(ops.ReportRequest{Scope: flags.scope(), AsOf: opsAsOf(reportAt)})
		skillScope, _ := json.Marshal(workgroup.SkillScope{Kind: "project", Team: flags.Team, Project: flags.Project})
		return o.submitFlow(command, root, c, control.Request{Operation: "ops_report", Workgroup: ops.Workgroup, Input: input, Answer: objective, SkillScope: skillScope, SkillCandidate: candidate, ManualCheckpoint: checkpoint})
	}}
	report.Flags().StringVar(&objective, "objective", "", "Team operations question or desired risk assessment")
	report.Flags().StringVar(&reportAt, "as-of", "", "RFC3339 report time; defaults to now")
	report.Flags().StringVar(&candidate, "skill-candidate", "", "Exact inactive scoped bundle for an isolated comparison")
	report.Flags().BoolVar(&checkpoint, "checkpoint", false, "Wait for owner approval between FLOW stages")
	_ = report.MarkFlagRequired("objective")
	reconcile := &cobra.Command{Use: "reconcile JOB_ID", Args: cobra.ExactArgs(1), Short: "Apply a verified AI report only to shadow incidents and notification plans", RunE: func(command *cobra.Command, args []string) error {
		return o.opsApply(command, flags.command("assessment.apply", args[0], nil), "ops_reconcile")
	}}
	apply := &cobra.Command{Use: "apply COMMAND_JSON", Args: cobra.ExactArgs(1), Short: "Apply an exact owner command with rich evidence references and stable retry identity", RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		data, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		var value ops.Command
		if err = mail.Decode(data, &value); err != nil {
			return err
		}
		if value.Scope != flags.scope() {
			return errors.New("command scope differs from the explicit CLI scope")
		}
		return o.opsApply(command, value, "ops_command")
	}}
	cmd.AddCommand(scope, work, release, observation, policy, incident, show, history, status, snapshot, sources, readReceipt, outbox, report, reconcile, apply)
	return cmd
}

func opsAsOf(value string) string {
	if value != "" {
		return value
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func (o *options) opsApply(command *cobra.Command, value ops.Command, operation string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	req := control.Request{Operation: operation, Input: data, JobID: value.EntityID}
	if handled, err := o.managed(command, req); handled || err != nil {
		if err != nil {
			return fmt.Errorf("OPS operation %s: %w", value.OperationID, err)
		}
		return nil
	}
	s, c, closeStore, err := o.open(command.Context(), true)
	if err != nil {
		return fmt.Errorf("OPS operation %s: %w", value.OperationID, err)
	}
	defer closeStore()
	result, err := (&runner.Runner{Store: s, Config: c}).Handle(command.Context(), req)
	if err != nil {
		return fmt.Errorf("OPS operation %s: %w", value.OperationID, err)
	}
	return output(command, result)
}
