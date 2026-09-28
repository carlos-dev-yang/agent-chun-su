// Package stagedworkflow builds bounded, versioned stages and their host-owned
// evidence contracts. The controller owns scheduling and artifact publication.
package stagedworkflow

import (
	"encoding/json"
	"errors"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

const Version = "staged-workflow-v1"
const WebWorkgroup = "web-research"

const (
	Collect    = "collect"
	Refine     = "refine"
	Synthesize = "synthesize"
	Validate   = "validate"
	Deliver    = "deliver"
)

type Origin struct {
	ConversationID   string
	MessageID        string
	RequestRevision  int64
	Destination      string
	ManualCheckpoint bool
}

// BuildSpec fixes stage dependencies and role identities before admission.
// An existing snapshot is validated and identified, never read from a path
// chosen by a model. The web source scope is public HTTPS only.
func BuildSpec(root, group, objective string, snapshot []byte, c config.Config, origin Origin) (store.WorkflowSpec, error) {
	var spec store.WorkflowSpec
	if strings.TrimSpace(objective) == "" || len(objective) > int(c.Limits.MaxArtifactBytes) {
		return spec, errors.New("staged objective is empty or exceeds the configured artifact budget")
	}
	if group != WebWorkgroup {
		if err := workgroup.ValidateInput(group, snapshot, c.Limits); err != nil {
			return spec, err
		}
	} else if len(snapshot) != 0 && string(snapshot) != "{}" {
		return spec, errors.New("web research does not accept a private saved snapshot")
	}
	request, _ := json.Marshal(objective)
	sourceIDs := []string{}
	bundleDigest := ""
	if group != WebWorkgroup {
		collected, err := snapshotCollection(group, snapshot, c.Limits)
		if err != nil {
			return spec, err
		}
		for _, source := range collected.Sources {
			sourceIDs = append(sourceIDs, source.ID)
		}
		_, bundleDigest, err = workgroup.ActiveFor(root, group, c.Limits.MaxArtifactBytes)
		if err != nil {
			return spec, err
		}
	}
	scope, _ := json.Marshal(map[string]any{"workgroup": group, "input_digest": files.Digest(snapshot), "source_ids": sourceIDs, "public_https_only": group == WebWorkgroup, "bundle_digest": bundleDigest, "mail_mode": c.MailMode})
	budget, _ := json.Marshal(c.Limits)
	destination := origin.Destination
	if destination == "" {
		destination = "local"
	}
	dest, _ := json.Marshal(destination)
	criteria, _ := json.Marshal(map[string]any{"required_stages": []string{Collect, Refine, Synthesize, Validate, Deliver}, "result": "validated artifact or explicit insufficient evidence"})
	spec = store.WorkflowSpec{
		OriginConversationID: origin.ConversationID, OriginMessageID: origin.MessageID,
		RequestRevision: origin.RequestRevision, OriginalRequest: request,
		CompletionCriteria: criteria, SourceScope: scope, Budget: budget,
		Destination: dest, WorkflowVersion: Version, ManualCheckpoint: origin.ManualCheckpoint,
	}
	role := map[string]string{Collect: config.RoleCollection, Refine: config.RoleRefinement, Synthesize: config.RoleSynthesis}
	for i, stage := range []string{Collect, Refine, Synthesize, Validate, Deliver} {
		step := store.StepSpec{Key: stage, Stage: stage, ExpectedOutputSchema: stage + "-v1", InstructionVersion: Version, GatePolicy: "auto"}
		if i > 0 {
			step.DependsOn = []string{[]string{Collect, Refine, Synthesize, Validate}[i-1]}
		}
		if stageRole := role[stage]; stageRole != "" {
			route := c.ExecutorFor(stageRole)
			step.ExecutorModel, step.ExecutorEffort = route.Model, route.ReasoningEffort
			step.ExecutorIdentity = RouteIdentity(route)
			if step.ExecutorModel == "" || step.ExecutorEffort == "" {
				return store.WorkflowSpec{}, errors.New("staged model and reasoning effort must be selected for each AI role")
			}
		} else {
			step.ExecutorIdentity = "host:" + stage + ":" + Version
		}
		spec.Steps = append(spec.Steps, step)
	}
	return spec, nil
}

// RouteIdentity covers every execution selector but excludes mutable grant
// flags. Source authorization is rechecked separately at each model call.
func RouteIdentity(route config.Executor) string {
	data, _ := json.Marshal(struct{ Kind, Path, Model, Effort, Environment string }{
		route.Kind, route.Path, route.Model, route.ReasoningEffort, route.Environment,
	})
	return files.Digest(data)
}

func ValidatePinnedRoute(role, model, effort, identity string, c config.Config) error {
	route := c.ExecutorFor(role)
	if route.Model != model || route.ReasoningEffort != effort || RouteIdentity(route) != identity {
		return errors.New("staged executor route changed after admission")
	}
	return nil
}
