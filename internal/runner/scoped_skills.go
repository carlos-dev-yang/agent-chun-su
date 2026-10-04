package runner

import (
	"encoding/json"
	"errors"

	"chunsu/internal/control"
	"chunsu/internal/mail"
	"chunsu/internal/workgroup"
)

// resolveSkills is an owner-management selection. Reception capabilities do
// not expose the scope/candidate fields; scope metadata never grants access.
func (r *Runner) resolveSkills(group string, req control.Request) (*workgroup.SkillScope, string, error) {
	if len(req.SkillScope) == 0 {
		if req.SkillCandidate != "" {
			return nil, "", errors.New("Skill candidate selection requires an explicit scope")
		}
		return nil, "", nil
	}
	var scope workgroup.SkillScope
	if err := mail.Decode(req.SkillScope, &scope); err != nil {
		return nil, "", err
	}
	if err := scope.Validate(); err != nil {
		return nil, "", err
	}
	// Initialization remains necessary even for inactive candidate execution.
	_, digest, err := workgroup.ActiveInScope(r.Store.Root, group, &scope, r.Config.Limits.MaxArtifactBytes)
	if err != nil {
		return nil, "", err
	}
	if req.SkillCandidate != "" {
		bundle, err := workgroup.LoadFor(r.Store.Root, group, req.SkillCandidate, r.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return nil, "", err
		}
		if err = workgroup.ValidateScopeBundle(bundle, scope); err != nil {
			return nil, "", err
		}
		digest = req.SkillCandidate
	}
	return &scope, digest, nil
}

func addSkillPin(request map[string]any, scope *workgroup.SkillScope, digest string) {
	if scope != nil {
		request["skill_scope"] = scope
		request["skill_bundle_digest"] = digest
	}
}

func scopedRequest(raw any, scope *workgroup.SkillScope, digest string) (map[string]any, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var request map[string]any
	if err = json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	if request == nil {
		request = map[string]any{}
	}
	addSkillPin(request, scope, digest)
	return request, nil
}
