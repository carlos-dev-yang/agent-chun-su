package cli

import (
	"errors"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/runner"
	"chunsu/internal/runtimeenv"
	"chunsu/internal/store"
	"github.com/spf13/cobra"
)

func (o *options) configModels() *cobra.Command {
	return &cobra.Command{Use: "models", Short: "List locally supported Codex model presets", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return output(cmd, executor.CodexModels())
	}}
}

func (o *options) configGrant() *cobra.Command {
	var allow bool
	var proof, policyDigest string
	cmd := &cobra.Command{Use: "grant ROLE SOURCE", Short: "Explicitly approve or revoke one role's private source disclosure", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		role, source := args[0], args[1]
		if role != config.RoleTask && role != config.RoleReception && role != config.RoleRefinement && role != config.RoleSynthesis && role != config.RoleReview {
			return errors.New("unsupported disclosure role")
		}
		if source != "mail" && source != "jira" && source != "code" {
			return errors.New("source must be mail, jira or code")
		}
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		lock, err := platform.Acquire(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		c, err = config.Load(root)
		if err != nil {
			return err
		}
		selected := c.ExecutorFor(role)
		if allow {
			if executor.Inspect(cmd.Context(), role, root, selected, c.Limits).Status != "prerequisites_match" {
				return errors.New("the exact model, effort, CLI and host boundary are not validated for this role")
			}
			if source == "jira" {
				if role != config.RoleTask && role != config.RoleRefinement && role != config.RoleSynthesis {
					return errors.New("Jira private handoff to this role has no source proof path; host delivery remains available")
				}
				if !files.ValidDigest(policyDigest) || !files.ValidID(proof) {
					return errors.New("Jira approval needs --policy-digest and --proof synthetic job ID")
				}
				s, openErr := store.OpenReadOnly(cmd.Context(), root)
				if openErr != nil {
					return openErr
				}
				if role == config.RoleTask {
					candidate := c
					candidate.Executor = selected
					candidate.Executor.LiveJiraPolicyDigest = policyDigest
					err = runner.VerifyJiraSyntheticProof(cmd.Context(), s, candidate, proof)
				} else {
					err = runner.VerifyStagedJiraProof(cmd.Context(), s, c, proof, role, policyDigest)
				}
				closeErr := s.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				selected.LiveJiraPolicyDigest, selected.LiveJiraValidationJobID = policyDigest, proof
			}
		}
		switch source {
		case "mail":
			selected.LiveMailApproved = allow
		case "code":
			selected.LiveCodeApproved = allow
		case "jira":
			selected.LiveJiraApproved = allow
		}
		selectedRole := &selected
		switch role {
		case config.RoleTask:
			c.Executor = selected
		case config.RoleReception:
			c.Routes.Reception = selectedRole
		case config.RoleRefinement:
			c.Routes.Refinement = selectedRole
		case config.RoleSynthesis:
			c.Routes.Synthesis = selectedRole
		case config.RoleReview:
			c.Routes.Review = selectedRole
		}
		if err = config.Save(root, c); err != nil {
			return err
		}
		return output(cmd, map[string]any{"role": role, "source": source, "approved": allow, "model": selected.Model, "reasoning_effort": selected.ReasoningEffort, "proof_job_id": selected.LiveJiraValidationJobID})
	}}
	cmd.Flags().BoolVar(&allow, "allow", false, "Approve this source for the selected role; omitted revokes")
	cmd.Flags().StringVar(&proof, "proof", "", "Completed synthetic Jira proof job ID")
	cmd.Flags().StringVar(&policyDigest, "policy-digest", "", "Selected Jira report policy SHA-256 digest")
	return cmd
}

func (o *options) configPolicy() *cobra.Command {
	cmd := &cobra.Command{Use: "policy", Short: "Inspect or explicitly apply the conversation model policy"}
	cmd.AddCommand(&cobra.Command{Use: "show", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		return output(cmd, map[string]any{"version": c.ModelPolicyVersion, "task": c.ExecutorFor(config.RoleTask), "reception": c.ExecutorFor(config.RoleReception), "collection": c.ExecutorFor(config.RoleCollection), "refinement": c.ExecutorFor(config.RoleRefinement), "synthesis": c.ExecutorFor(config.RoleSynthesis), "review": c.ExecutorFor(config.RoleReview)})
	}})
	cmd.AddCommand(&cobra.Command{Use: "apply", Short: "Select Sol medium chat, Luna xhigh evidence, and Sol xhigh synthesis; revoke changed-route disclosure grants", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		lock, err := platform.Acquire(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		c, err = config.Load(root)
		if err != nil {
			return err
		}
		if err = c.ApplyConversationPolicy(); err != nil {
			return err
		}
		if err = config.Save(root, c); err != nil {
			return err
		}
		return output(cmd, map[string]any{"version": c.ModelPolicyVersion, "status": "configured_unvalidated", "disclosures": "revoked", "compatibility": "run doctor and synthetic boundary validation for this executable"})
	}})
	return cmd
}

func (o *options) configRoute() *cobra.Command {
	var driver, path, model, environment, effort string
	var inherit bool
	cmd := &cobra.Command{Use: "route ROLE", Short: "Select a role's execution identity independently", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		role := args[0]
		if role != config.RoleTask && role != config.RoleReception && role != config.RoleReview && role != config.RoleCollection && role != config.RoleRefinement && role != config.RoleSynthesis {
			return errors.New("unsupported role")
		}
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		lock, err := platform.Acquire(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second)
		if err != nil {
			return err
		}
		defer lock.Close()
		c, err = config.Load(root)
		if err != nil {
			return err
		}
		if inherit {
			if role == config.RoleTask || driver != "" || path != "" || model != "" || environment != "" || effort != "" {
				return errors.New("inherit applies only to a non-task role and cannot be combined with route fields")
			}
			switch role {
			case config.RoleReception:
				c.Routes.Reception = nil
			case config.RoleReview:
				c.Routes.Review = nil
			case config.RoleCollection:
				c.Routes.Collection = nil
			case config.RoleRefinement:
				c.Routes.Refinement = nil
			case config.RoleSynthesis:
				c.Routes.Synthesis = nil
			}
		} else {
			if driver == "" && path == "" && model == "" && environment == "" && effort == "" {
				return output(cmd, map[string]any{"role": role, "executor": c.ExecutorFor(role)})
			}
			selected := c.ExecutorFor(role)
			if driver != "" {
				selected.Kind = driver
			}
			if path != "" {
				selected.Path = path
			}
			if model != "" {
				selected.Model = model
			}
			if environment != "" {
				selected.Environment = environment
			}
			if effort != "" {
				if !config.ValidEffort(effort) {
					return errors.New("unsupported reasoning effort")
				}
				selected.ReasoningEffort = effort
			}
			if _, err = executor.Select(selected.Kind); err != nil {
				return err
			}
			if _, err = runtimeenv.Select(selected.Environment); err != nil {
				return err
			}
			selected.RevokeDisclosure()
			switch role {
			case config.RoleTask:
				c.Executor = selected
			case config.RoleReception:
				c.Routes.Reception = &selected
			case config.RoleReview:
				c.Routes.Review = &selected
			case config.RoleCollection:
				c.Routes.Collection = &selected
			case config.RoleRefinement:
				c.Routes.Refinement = &selected
			case config.RoleSynthesis:
				c.Routes.Synthesis = &selected
			}
		}
		if err = config.Save(root, c); err != nil {
			return err
		}
		return output(cmd, map[string]any{"role": role, "executor": c.ExecutorFor(role), "inherit_task": inherit, "disclosure": "an explicitly changed route requires its own live-data approval"})
	}}
	cmd.Flags().StringVar(&driver, "driver", "", "Compiled AI driver")
	cmd.Flags().StringVar(&path, "path", "", "Driver executable path")
	cmd.Flags().StringVar(&model, "model", "", "Model selected for this role")
	cmd.Flags().StringVar(&environment, "environment", "", "Execution environment adapter")
	cmd.Flags().StringVar(&effort, "effort", "", "Reasoning effort for the selected model")
	cmd.Flags().BoolVar(&inherit, "inherit", false, "Use task route again (reception/review only)")
	return cmd
}
