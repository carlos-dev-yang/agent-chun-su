package cli

import (
	"encoding/json"

	"chunsu/internal/feedback"
	"github.com/spf13/cobra"
)

func (o *options) review() *cobra.Command {
	cmd := &cobra.Command{Use: "review", Short: "Review explicitly selected saved results; never runs automatically after a task"}
	var caseID, rubricID, skillID string
	evaluate := &cobra.Command{Use: "evaluate JOB_ID", Short: "Run an isolated evaluation using pinned expectations, rubric and Skill", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		result, err := (feedback.Service{Store: s, Config: c}).Evaluate(cmd.Context(), args[0], caseID, rubricID, skillID)
		if err != nil {
			if result.Request.ID != "" {
				_ = output(cmd, map[string]any{"review": result, "status": "not_accepted"})
			}
			return err
		}
		return output(cmd, result)
	}}
	evaluate.Flags().StringVar(&caseID, "case", "", "Immutable case record ID matching the job input")
	evaluate.Flags().StringVar(&rubricID, "rubric", "", "Immutable rubric record ID")
	evaluate.Flags().StringVar(&skillID, "evaluator-skill", "", "Independent evaluator Skill record ID")
	var criteriaID, question string
	analyze := &cobra.Command{Use: "analyze JOB_ID...", Short: "Run selected analyzer or optimizer criteria over accumulated results", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		result, err := (feedback.Service{Store: s, Config: c}).Analyze(cmd.Context(), criteriaID, question, args)
		if err != nil {
			if result.Request.ID != "" {
				_ = output(cmd, map[string]any{"review": result, "status": "not_accepted"})
			}
			return err
		}
		return output(cmd, result)
	}}
	analyze.Flags().StringVar(&criteriaID, "criteria", "", "Immutable review_criteria record ID (analyzer or optimizer)")
	analyze.Flags().StringVar(&question, "question", "", "Bounded question for this selected set of results")
	var criteriaName, criteriaGroup, criteriaPurpose, criteriaReviewer, criteriaStatus string
	pin := &cobra.Command{Use: "pin SKILL.md", Short: "Preserve analyzer or optimizer criteria without activating or running them", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, c, close, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer close()
		content, err := readExternal(args[0], c.Limits.MaxArtifactBytes)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(feedback.ReviewCriteria{Version: feedback.Version, Name: criteriaName, Workgroup: criteriaGroup, Purpose: criteriaPurpose, ReviewStatus: criteriaStatus, Reviewer: criteriaReviewer, Skill: string(content)})
		if err != nil {
			return err
		}
		record, err := (feedback.Service{Store: s, Config: c}).Add(cmd.Context(), "review_criteria", "", payload)
		if err != nil {
			return err
		}
		return output(cmd, record)
	}}
	pin.Flags().StringVar(&criteriaName, "name", "", "Skill name matching its front matter")
	pin.Flags().StringVar(&criteriaGroup, "workgroup", "", "Workgroup whose results may be reviewed")
	pin.Flags().StringVar(&criteriaPurpose, "purpose", "", "analyzer or optimizer")
	pin.Flags().StringVar(&criteriaReviewer, "reviewer", "", "Author or human reviewer of these criteria")
	pin.Flags().StringVar(&criteriaStatus, "review-status", "draft", "draft, validation or human_reviewed")
	cmd.AddCommand(evaluate, analyze, pin)
	return cmd
}
