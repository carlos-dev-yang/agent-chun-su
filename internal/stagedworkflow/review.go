package stagedworkflow

import (
	"encoding/json"
	"errors"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/ops"
	"chunsu/internal/workgroup"
)

type ModelContract struct {
	PromptDigest string
	SchemaDigest string
	SkillDigest  string
}

// ModelContracts uses the same preparation functions as actual execution.
// Self-consistent audit hashes alone cannot prove the admitted objective,
// source presentation, role instructions or output contract were submitted.
func ModelContracts(group, objective string, rawData, evidenceData []byte, limits config.Limits, bundle workgroup.Bundle) (map[string]ModelContract, error) {
	var raw RawBundle
	if err := mail.Decode(rawData, &raw); err != nil {
		return nil, err
	}
	if raw.Workgroup != group {
		return nil, errors.New("model contract workgroup differs from collected evidence")
	}
	if _, err := validateRawBundle(raw, limits); err != nil {
		return nil, err
	}
	evidence, err := validateEvidence(raw, evidenceData, limits)
	if err != nil {
		return nil, err
	}
	contracts := map[string]ModelContract{}
	contract := func(prompt, schema, skill []byte) ModelContract {
		return ModelContract{PromptDigest: files.Digest(prompt), SchemaDigest: files.Digest(schema), SkillDigest: files.Digest(skill)}
	}
	if group != ops.Workgroup {
		prompt, schema, skill, err := refinementModelInput(raw, objective)
		if err != nil {
			return nil, err
		}
		contracts[config.RoleRefinement] = contract(prompt, schema, skill)
	}
	prompt, schema, skill, _, err := synthesisModelInput(group, objective, evidence, bundle)
	if err != nil {
		return nil, err
	}
	contracts[config.RoleSynthesis] = contract(prompt, schema, skill)
	return contracts, nil
}

// VerifyResultEvidence rechecks the exact persisted collect -> refine ->
// synthesis -> validation chain without running a model or using active state.
// It is not a replacement for independent semantic evaluation.
func VerifyResultEvidence(group string, snapshot []byte, limits config.Limits, bundle workgroup.Bundle, mode string, rawData, evidenceData, synthesisData, validatedData, refinementResponse, synthesisResponse []byte) (Validated, error) {
	var raw RawBundle
	var synthesis Synthesis
	var actual Validated
	if (bundle.Workgroup != group && !(bundle.Workgroup == "" && group == mail.Workgroup)) || group == WebWorkgroup {
		return actual, errors.New("independent report review requires a compiled domain bundle")
	}
	for _, data := range [][]byte{snapshot, rawData, evidenceData, synthesisData, validatedData, refinementResponse, synthesisResponse} {
		if int64(len(data)) > limits.MaxArtifactBytes {
			return actual, errors.New("staged review artifact exceeds configured budget")
		}
	}
	if err := bundle.Validate(); err != nil {
		return actual, err
	}
	if err := mail.Decode(rawData, &raw); err != nil {
		return actual, err
	}
	expectedRaw, err := snapshotCollectionVersion(group, snapshot, limits, raw.ProjectionVersion)
	if err != nil {
		return actual, err
	}
	left, _ := json.Marshal(raw)
	right, _ := json.Marshal(expectedRaw)
	if files.Digest(left) != files.Digest(right) {
		return actual, errors.New("collected evidence differs from admitted immutable input")
	}
	evidence, err := validateEvidence(expectedRaw, evidenceData, limits)
	if err != nil {
		return actual, err
	}
	var rebuilt []byte
	if group == ops.Workgroup {
		if len(refinementResponse) != 0 {
			return actual, errors.New("deterministic team-ops refinement cannot claim a model response")
		}
		rebuilt, _, err = ProjectOpsEvidence(expectedRaw, limits)
	} else {
		var proposal struct {
			Sources []SourceFacts `json:"sources"`
		}
		if err = mail.Decode(refinementResponse, &proposal); err != nil {
			return actual, err
		}
		rebuilt, err = buildEvidence(expectedRaw, proposal.Sources, limits)
	}
	if err != nil {
		return actual, err
	}
	if !samePinnedJSON(rebuilt, evidenceData) {
		return actual, errors.New("refinement differs from its preserved execution evidence")
	}
	if err = mail.Decode(synthesisData, &synthesis); err != nil {
		return actual, err
	}
	if synthesis.Version != 1 || synthesis.Workgroup != group || synthesis.EvidenceDigest != files.Digest(evidenceData) || !json.Valid(synthesis.Report) {
		return actual, errors.New("synthesis is not bound to the selected refinement")
	}
	if !samePinnedJSON(synthesis.Report, synthesisResponse) {
		return actual, errors.New("synthesis differs from its preserved model response")
	}
	left, _ = json.Marshal(synthesis.Evidence)
	right, _ = json.Marshal(evidence)
	if files.Digest(left) != files.Digest(right) {
		return actual, errors.New("synthesis changed the verified evidence")
	}
	expected, err := validateDomainSynthesis(group, snapshot, limits, bundle, mode, synthesis)
	if err != nil {
		return actual, err
	}
	if err = mail.Decode(validatedData, &actual); err != nil {
		return actual, err
	}
	left, _ = json.Marshal(actual)
	right, _ = json.Marshal(expected)
	if files.Digest(left) != files.Digest(right) {
		return actual, errors.New("validated report differs from its pinned synthesis")
	}
	return actual, nil
}
