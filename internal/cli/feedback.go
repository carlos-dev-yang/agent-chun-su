package cli

import (
	"encoding/json"
	"errors"

	"chunsu/internal/feedback"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
	"github.com/spf13/cobra"
)

func (o *options) feedback() *cobra.Command {
	cmd := &cobra.Command{Use: "feedback", Short: "Store case expectations, independent evaluations and optimization findings"}
	var job, kind, evaluatorSkillID, evaluatorSkillHash string
	var limit int
	add := &cobra.Command{Use: "import KIND RECORD.json", Short: "Import case, rubric, evaluation, feedback or finding as an immutable record", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		data, err := readExternal(args[1], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		if args[0] == "evaluation" && (evaluatorSkillID != "" || evaluatorSkillHash != "") {
			var evaluation feedback.Evaluation
			if err = json.Unmarshal(data, &evaluation); err != nil {
				return err
			}
			evaluation.EvaluatorSkillID = evaluatorSkillID
			evaluation.EvaluatorSkillHash = evaluatorSkillHash
			data, err = json.Marshal(evaluation)
			if err != nil {
				return err
			}
		}
		record, err := (feedback.Service{Store: s, Config: c}).Add(cmd.Context(), args[0], job, data)
		if err != nil {
			return err
		}
		return output(cmd, record)
	}}
	add.Flags().StringVar(&job, "job", "", "Job whose result is being evaluated")
	add.Flags().StringVar(&evaluatorSkillID, "evaluator-skill", "", "Pinned evaluator Skill record ID for an evaluation import")
	add.Flags().StringVar(&evaluatorSkillHash, "evaluator-skill-hash", "", "Exact content hash returned by pin-evaluator")
	var evaluatorName, evaluatorWorkgroup string
	pinEvaluator := &cobra.Command{Use: "pin-evaluator SKILL.md", Short: "Preserve an independent evaluator Skill for later evaluation imports", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		content, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(feedback.EvaluatorSkill{Version: feedback.Version, Name: evaluatorName, Workgroup: evaluatorWorkgroup, Content: string(content)})
		if err != nil {
			return err
		}
		record, err := (feedback.Service{Store: s, Config: c}).Add(cmd.Context(), "evaluator_skill", "", payload)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"record": record, "content_hash": files.Digest(content), "note": "This preserves instructions for imported independent judgments; it does not run an evaluator."})
	}}
	pinEvaluator.Flags().StringVar(&evaluatorName, "name", "golden-evaluation", "Human-readable evaluator Skill name")
	pinEvaluator.Flags().StringVar(&evaluatorWorkgroup, "workgroup", "", "Workgroup this evaluator Skill may judge")
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, _, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		records, err := s.Records(cmd.Context(), kind, "", limit)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"records": records, "limit": limit, "possibly_truncated": len(records) == limit})
	}}
	list.Flags().StringVar(&kind, "kind", "", "Filter record kind")
	list.Flags().IntVar(&limit, "limit", store.DefaultRecordLimit, "Maximum number of records")
	show := &cobra.Command{Use: "show RECORD_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		record, err := s.Record(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		data, err := s.ReadRecord(record, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"record": record, "content": json.RawMessage(data)})
	}}
	compare := &cobra.Command{Use: "compare BASELINE_JOB CANDIDATE_JOB", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		record, comparison, err := (feedback.Service{Store: s, Config: c}).Compare(cmd.Context(), args[0], args[1])
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"record": record, "comparison": comparison})
	}}
	cmd.AddCommand(add, pinEvaluator, list, show, compare)
	return cmd
}

func (o *options) workgroup() *cobra.Command {
	cmd := &cobra.Command{Use: "workgroup", Short: "Review candidate controls and record explicit adoption or rollback"}
	var activeWorkgroup string
	show := &cobra.Command{Use: "show", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		b, digest, err := workgroup.ActiveFor(s.Root, activeWorkgroup, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		path, err := workgroup.ActivePathFor(activeWorkgroup)
		if err != nil {
			return err
		}
		selection, err := readExternal(s.Root+"/"+path, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"digest": digest, "selection": json.RawMessage(selection), "bundle": b})
	}}
	show.Flags().StringVar(&activeWorkgroup, "workgroup", mail.Workgroup, "Workgroup whose active controls to show")
	cmd.AddCommand(show)
	var guide, schema, hypothesis, workgroupID string
	var evidence, checks []string
	propose := &cobra.Command{Use: "propose", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		bundle, _, err := workgroup.ActiveFor(s.Root, workgroupID, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		if guide == "" && schema == "" {
			return errors.New("provide a changed guide or schema file")
		}
		if guide != "" {
			data, e := readExternal(guide, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
			}
			if e = bundle.SetGuide(data); e != nil {
				return e
			}
		}
		if schema != "" {
			data, e := readExternal(schema, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
			}
			bundle.Schema = data
		}
		record, err := (feedback.Service{Store: s, Config: c}).Propose(cmd.Context(), bundle, evidence, hypothesis, checks)
		if err != nil {
			return err
		}
		return output(cmd, record)
	}}
	propose.Flags().StringVar(&guide, "guide", "", "Candidate public guide file")
	propose.Flags().StringVar(&schema, "schema", "", "Candidate result schema file")
	propose.Flags().StringVar(&workgroupID, "workgroup", mail.Workgroup, "Workgroup whose active controls this proposal changes")
	propose.Flags().StringVar(&hypothesis, "hypothesis", "", "Evidence-based expected improvement")
	propose.Flags().StringArrayVar(&evidence, "evidence", nil, "Supporting immutable record ID (repeatable)")
	propose.Flags().StringArrayVar(&checks, "check", nil, "Required comparison or regression check (repeatable)")
	diff := &cobra.Command{Use: "diff PROPOSAL_ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), false)
		if err != nil {
			return err
		}
		defer close()
		result, err := (feedback.Service{Store: s, Config: c}).Diff(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return output(cmd, result)
	}}
	var action, actor, actorKind, reason, comparison string
	decide := &cobra.Command{Use: "decide PROPOSAL_ID", Short: "Apply a human adoption/rejection/rollback decision; evaluation cannot call this tool", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		record, err := (feedback.Service{Store: s, Config: c}).Decide(cmd.Context(), args[0], action, actor, actorKind, reason, comparison)
		if err != nil {
			return err
		}
		return output(cmd, record)
	}}
	decide.Flags().StringVar(&action, "action", "", "adopt, reject, or rollback")
	decide.Flags().StringVar(&actor, "actor", "", "Person making the decision")
	decide.Flags().StringVar(&actorKind, "actor-kind", "human", "human or validation; validation is never evidence of personal approval")
	decide.Flags().StringVar(&reason, "reason", "", "Decision rationale and remaining limitations")
	decide.Flags().StringVar(&comparison, "comparison", "", "Comparable baseline/candidate record required for adoption")
	cmd.AddCommand(propose, diff, decide)
	return cmd
}

func (o *options) experiment() *cobra.Command {
	var candidate string
	cmd := &cobra.Command{Use: "experiment BASELINE_JOB", Short: "Queue a separate experiment using the same preserved input", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !mail.Nonempty(candidate) {
			return errors.New("candidate digest is required")
		}
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		j, err := s.Job(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if _, err = workgroup.LoadFor(s.Root, j.Workgroup, candidate, c.Limits.MaxArtifactBytes); err != nil {
			return err
		}
		art, err := s.InputArtifact(cmd.Context(), j)
		if err != nil {
			return err
		}
		input, err := s.ReadArtifact(art, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		var request map[string]any
		if err = json.Unmarshal(j.Request, &request); err != nil {
			return err
		}
		if request == nil {
			request = map[string]any{}
		}
		request["experiment_of"] = j.ID
		request["candidate_digest"] = candidate
		if acquisition, ok := request["acquisition_id"]; ok {
			request["source_acquisition_id"] = acquisition
			delete(request, "acquisition_id")
		}
		created, err := s.Submit(cmd.Context(), j.Workgroup, input, request, c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		return output(cmd, created)
	}}
	cmd.Flags().StringVar(&candidate, "candidate", "", "Stored workgroup digest to compare without adoption")
	return cmd
}
