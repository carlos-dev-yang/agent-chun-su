package cli

import (
	"encoding/json"
	"errors"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/feedback"
	"chunsu/internal/mail"
	"chunsu/internal/skills"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
	"github.com/spf13/cobra"
)

type skillFlags struct {
	Kind, Team, Project, Candidate string
}

func (f *skillFlags) add(cmd *cobra.Command, candidate bool) {
	cmd.Flags().StringVar(&f.Kind, "skill-scope", "", "Explicit common, team or project Skill selection namespace")
	cmd.Flags().StringVar(&f.Team, "team", "", "Team ID for a team/project Skill scope")
	cmd.Flags().StringVar(&f.Project, "project", "", "Project ID within the team")
	if candidate {
		cmd.Flags().StringVar(&f.Candidate, "skill-candidate", "", "Exact stored bundle for an inactive scoped comparison")
	}
}

func (f skillFlags) scope() (*workgroup.SkillScope, error) {
	if f.Kind == "" && f.Team == "" && f.Project == "" && f.Candidate == "" {
		return nil, nil
	}
	scope := workgroup.SkillScope{Kind: f.Kind, Team: f.Team, Project: f.Project}
	return &scope, scope.Validate()
}

func (f skillFlags) apply(req *control.Request) error {
	scope, err := f.scope()
	if err != nil || scope == nil {
		return err
	}
	req.SkillScope, err = json.Marshal(scope)
	req.SkillCandidate = f.Candidate
	return err
}

func (o *options) skills() *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "Manage scoped internal Skills; use workgroup assess/decide for reviewed adoption"}
	var actor, reason string
	var evidence []string
	var sanitized bool
	importSkill := &cobra.Command{Use: "import SKILL.json", Short: "Preserve an inactive common/team/project instruction version", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, c, closeStore, err := o.open(command.Context(), true)
		if err != nil {
			return err
		}
		defer closeStore()
		data, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		var skill workgroup.ScopedSkill
		if err = mail.Decode(data, &skill); err != nil {
			return err
		}
		record, digest, err := (skills.Service{Store: s, Config: c}).Register(command.Context(), skill, actor, reason, evidence, sanitized)
		if err != nil {
			return err
		}
		return output(command, map[string]any{"record": record, "skill_digest": digest, "activation": "inactive instruction asset"})
	}}
	importSkill.Flags().StringVar(&actor, "actor", "", "Owner registering this instruction version")
	importSkill.Flags().StringVar(&reason, "reason", "", "Change or reuse rationale (host-only provenance)")
	importSkill.Flags().StringArrayVar(&evidence, "evidence", nil, "Existing feedback/finding/evaluation record ID (repeatable)")
	importSkill.Flags().BoolVar(&sanitized, "sanitized-for-scope", false, "Owner declares content minimized and suitable for its reuse scope; required for common")
	_ = importSkill.MarkFlagRequired("actor")
	_ = importSkill.MarkFlagRequired("reason")
	show := &cobra.Command{Use: "show SKILL_DIGEST", Short: "Read an exact verified instruction version", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		skill, err := workgroup.LoadScopedSkill(root, args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(command, map[string]any{"digest": args[0], "skill": skill})
	}}
	var listKind string
	var limit int
	list := &cobra.Command{Use: "list", Short: "List immutable registrations, feedback or owner decisions", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		if listKind != "skill_registration" && listKind != "skill_feedback" && listKind != "decision" && listKind != "proposal" {
			return errors.New("skills list kind must be skill_registration, skill_feedback, decision or proposal")
		}
		s, c, closeStore, err := o.open(command.Context(), false)
		if err != nil {
			return err
		}
		defer closeStore()
		records, err := s.Records(command.Context(), listKind, "", limit)
		if err != nil {
			return err
		}
		items := []any{}
		for _, record := range records {
			data, err := s.ReadRecord(record, c.Limits.MaxArtifactBytes)
			if err != nil {
				return err
			}
			items = append(items, map[string]any{"record": record, "content": json.RawMessage(data)})
		}
		return output(command, map[string]any{"items": items, "possibly_truncated": len(records) == limit})
	}}
	list.Flags().StringVar(&listKind, "kind", "skill_registration", "Record kind to list")
	list.Flags().IntVar(&limit, "limit", store.DefaultRecordLimit, "Maximum records")
	var initial skillFlags
	var initialGroup, initialActor, initialReason string
	initialize := &cobra.Command{Use: "initialize", Short: "Snapshot the existing selected domain baseline into an empty scope", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		scope, err := initial.scope()
		if err != nil {
			return err
		}
		if scope == nil {
			return errors.New("initialize requires an explicit --skill-scope")
		}
		s, c, closeStore, err := o.open(command.Context(), true)
		if err != nil {
			return err
		}
		defer closeStore()
		record, err := (skills.Service{Store: s, Config: c}).Initialize(command.Context(), initialGroup, *scope, initialActor, initialReason)
		if err != nil {
			return err
		}
		return output(command, map[string]any{"record": record, "note": "Existing domain baseline selected for this scope; this is not new quality evidence."})
	}}
	initial.add(initialize, false)
	initialize.Flags().StringVar(&initialGroup, "workgroup", mail.Workgroup, "Compiled workgroup to initialize")
	initialize.Flags().StringVar(&initialActor, "actor", "", "Owner selecting the baseline")
	initialize.Flags().StringVar(&initialReason, "reason", "", "Baseline selection rationale")
	_ = initialize.MarkFlagRequired("actor")
	_ = initialize.MarkFlagRequired("reason")
	var activeFlags skillFlags
	var activeGroup string
	active := &cobra.Command{Use: "active", Short: "Read the exact scoped selection and component snapshots", Args: cobra.NoArgs, RunE: func(command *cobra.Command, args []string) error {
		scope, err := activeFlags.scope()
		if err != nil {
			return err
		}
		if scope == nil {
			return errors.New("active requires an explicit --skill-scope")
		}
		s, c, closeStore, err := o.open(command.Context(), false)
		if err != nil {
			return err
		}
		defer closeStore()
		bundle, digest, err := workgroup.ActiveInScope(s.Root, activeGroup, scope, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(command, map[string]any{"scope": scope, "bundle_digest": digest, "bundle": bundle})
	}}
	activeFlags.add(active, false)
	active.Flags().StringVar(&activeGroup, "workgroup", mail.Workgroup, "Compiled workgroup")
	var proposalEvidence, checks []string
	var hypothesis string
	compose := func(propose bool) *cobra.Command {
		use, short := "preview COMPOSITION.json", "Preview a pinned composition and its executor instructions"
		if propose {
			use, short = "propose COMPOSITION.json", "Create an inactive scoped candidate for existing comparison/release gates"
		}
		command := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
			s, c, closeStore, err := o.open(command.Context(), propose)
			if err != nil {
				return err
			}
			defer closeStore()
			data, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
			if err != nil {
				return err
			}
			var request skills.ComposeRequest
			if err = mail.Decode(data, &request); err != nil {
				return err
			}
			bundle, err := (skills.Service{Store: s, Config: c}).Compose(command.Context(), request)
			if err != nil {
				return err
			}
			if !propose {
				return output(command, map[string]any{"bundle": bundle, "executor_skill": bundle.Skill, "activation": "unchanged", "conflicts": "structured review-rule conflicts rejected; prose needs human review"})
			}
			record, err := (feedback.Service{Store: s, Config: c}).Propose(command.Context(), bundle, proposalEvidence, hypothesis, checks)
			if err != nil {
				return err
			}
			return output(command, record)
		}}
		if propose {
			command.Flags().StringVar(&hypothesis, "hypothesis", "", "Narrow expected improvement")
			command.Flags().StringArrayVar(&proposalEvidence, "evidence", nil, "Preserved feedback/finding/evaluation record ID (repeatable)")
			command.Flags().StringArrayVar(&checks, "check", nil, "Required comparison/regression check (repeatable)")
		}
		return command
	}
	var feedbackKind, observation, correction, feedbackActor string
	addFeedback := &cobra.Command{Use: "feedback JOB_ID", Short: "Record a scoped run correction without changing any active instruction", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		s, c, closeStore, err := o.open(command.Context(), true)
		if err != nil {
			return err
		}
		defer closeStore()
		record, err := (skills.Service{Store: s, Config: c}).RecordFeedback(command.Context(), args[0], feedbackKind, observation, correction, feedbackActor)
		if err != nil {
			return err
		}
		return output(command, record)
	}}
	addFeedback.Flags().StringVar(&feedbackKind, "kind", "", "false-positive, omission, severity, owner, duplicate, insufficient-evidence or useful")
	addFeedback.Flags().StringVar(&observation, "observation", "", "Observed result issue or usefulness")
	addFeedback.Flags().StringVar(&correction, "correction", "", "Corrected interpretation and evidence; required for an issue")
	addFeedback.Flags().StringVar(&feedbackActor, "actor", "", "Owner recording the feedback")
	cmd.AddCommand(importSkill, show, list, initialize, active, compose(false), compose(true), addFeedback)
	return cmd
}
