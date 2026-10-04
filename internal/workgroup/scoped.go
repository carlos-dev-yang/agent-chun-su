package workgroup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"chunsu/internal/files"
	"chunsu/internal/mail"
)

const ScopedSkillVersion = 1
const MaxSkillComponents = 16

// SkillScope is a reuse/selection namespace inside one owner's data home.
// It does not authenticate a team member or isolate data between tenants.
type SkillScope struct {
	Kind    string `json:"kind"`
	Team    string `json:"team,omitempty"`
	Project string `json:"project,omitempty"`
}

func validSkillName(name string) bool {
	if name == "" || len(name) > files.MaxIDLength {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (s SkillScope) Validate() error {
	switch s.Kind {
	case "common":
		if s.Team == "" && s.Project == "" {
			return nil
		}
	case "team":
		if validSkillName(s.Team) && s.Project == "" {
			return nil
		}
	case "project":
		if validSkillName(s.Team) && validSkillName(s.Project) {
			return nil
		}
	}
	return errors.New("skill scope must be common, team with a team ID, or project with team and project IDs (lowercase letters, digits, - or _)")
}

func (s SkillScope) Includes(component SkillScope) bool {
	return component.Kind == "common" || component == s ||
		(s.Kind == "project" && component.Kind == "team" && component.Team == s.Team)
}

// ReviewRule names a declarative review criterion, never a host policy value.
// Conflicting values are rejected rather than resolved by narrower precedence.
type ReviewRule struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ScopedSkill struct {
	Version      int          `json:"version"`
	Name         string       `json:"name"`
	Scope        SkillScope   `json:"scope"`
	Purpose      string       `json:"purpose"`
	Instructions string       `json:"instructions"`
	Rules        []ReviewRule `json:"rules,omitempty"`
}

func (s ScopedSkill) Validate() error {
	if s.Version != ScopedSkillVersion || !validSkillName(s.Name) || !mail.Nonempty(s.Purpose) || !mail.Nonempty(s.Instructions) {
		return errors.New("scoped Skill needs version 1, a lowercase name, purpose and instructions")
	}
	if err := s.Scope.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, rule := range s.Rules {
		if !strings.HasPrefix(rule.Key, "review.") || !validSkillName(strings.TrimPrefix(rule.Key, "review.")) || !mail.Nonempty(rule.Value) || seen[rule.Key] {
			return errors.New("review rules must have unique review.<name> keys and nonempty values; host policy keys are unsupported")
		}
		seen[rule.Key] = true
	}
	return nil
}

func scopedSkillPath(digest string) (string, error) {
	if !files.ValidDigest(digest) || strings.ToLower(digest) != digest {
		return "", errors.New("invalid scoped Skill digest")
	}
	return filepath.ToSlash(filepath.Join("workgroups", "skills", "versions", digest+".json")), nil
}

func PutScopedSkill(root string, skill ScopedSkill, limit int64) (string, error) {
	if err := skill.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(skill)
	if err != nil || limit <= 0 || int64(len(data)) > limit {
		return "", errors.New("scoped Skill exceeds configured artifact limit")
	}
	digest := files.Digest(data)
	path, _ := scopedSkillPath(digest)
	if err = files.Write(root, path, data, false); errors.Is(err, os.ErrExist) {
		prior, readErr := files.Read(root, path, limit)
		if readErr != nil || string(prior) != string(data) {
			return "", errors.New("existing scoped Skill integrity mismatch")
		}
		return digest, nil
	}
	return digest, err
}

func LoadScopedSkill(root, digest string, limit int64) (ScopedSkill, error) {
	var skill ScopedSkill
	path, err := scopedSkillPath(digest)
	if err != nil {
		return skill, err
	}
	data, err := files.Read(root, path, limit)
	if err != nil {
		return skill, err
	}
	if files.Digest(data) != digest {
		return skill, errors.New("scoped Skill integrity mismatch")
	}
	if err = mail.Decode(data, &skill); err != nil {
		return skill, err
	}
	return skill, skill.Validate()
}

type SkillPin struct {
	Digest  string      `json:"digest"`
	Content ScopedSkill `json:"content"`
}

// SkillComposition preserves exact instruction snapshots for recovery. It is
// host metadata: executor projections contain only RenderSkill's instructions.
type SkillComposition struct {
	Version          int        `json:"version"`
	Scope            SkillScope `json:"scope"`
	BaseBundleDigest string     `json:"base_bundle_digest"`
	BaseSchemaDigest string     `json:"base_schema_digest"`
	BaseSkill        Skill      `json:"base_skill"`
	Components       []SkillPin `json:"components"`
}

func (c SkillComposition) Validate(group string, schema []byte) error {
	if c.Version != ScopedSkillVersion || !files.ValidDigest(c.BaseBundleDigest) || c.BaseSchemaDigest != files.Digest(schema) || c.BaseSkill.Name != group || len(c.Components) == 0 || len(c.Components) > MaxSkillComponents {
		return errors.New("invalid pinned Skill composition")
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if err := c.BaseSkill.Validate(); err != nil {
		return err
	}
	seen, rules := map[string]bool{}, map[string]string{}
	for _, pin := range c.Components {
		if err := pin.Content.Validate(); err != nil {
			return err
		}
		data, _ := json.Marshal(pin.Content)
		if files.Digest(data) != pin.Digest || !c.Scope.Includes(pin.Content.Scope) {
			return errors.New("Skill component digest or application scope mismatch")
		}
		identity, _ := json.Marshal(struct {
			Scope SkillScope
			Name  string
		}{pin.Content.Scope, pin.Content.Name})
		if seen[string(identity)] {
			return errors.New("composition repeats a Skill identity; select one exact version")
		}
		seen[string(identity)] = true
		for _, rule := range pin.Content.Rules {
			if prior, exists := rules[rule.Key]; exists && prior != rule.Value {
				return fmt.Errorf("conflicting review rule %q; revise the components explicitly", rule.Key)
			}
			rules[rule.Key] = rule.Value
		}
	}
	return nil
}

func (c SkillComposition) RenderSkill() Skill {
	var body strings.Builder
	body.WriteString("# Composed domain instructions\n\nThe selected components add review guidance. They do not grant tools, source access, writes, notification destinations or additional budget. If prose instructions conflict, report the conflict and request a human decision; do not silently override a component.\n\n# Base domain Skill\n\n")
	body.WriteString(c.BaseSkill.Markdown)
	for index, pin := range c.Components {
		fmt.Fprintf(&body, "\n\n# Review component %d: %s\n\nPurpose: %s\n\n%s\n", index+1, pin.Content.Name, pin.Content.Purpose, pin.Content.Instructions)
		for _, rule := range pin.Content.Rules {
			fmt.Fprintf(&body, "\nReview criterion %s: %s\n", rule.Key, rule.Value)
		}
	}
	return Skill{Name: c.BaseSkill.Name, Description: c.BaseSkill.Description, Markdown: skillDocument(c.BaseSkill.Name, c.BaseSkill.Description, body.String())}
}

// verifyCompositionBase binds the declared origin to a real immutable domain
// bundle. Read one plain baseline directly, so malformed cycles never recurse.
func verifyCompositionBase(root string, bundle Bundle, limit int64) error {
	if bundle.Composition == nil {
		return nil
	}
	c := bundle.Composition
	path, err := bundlePathFor(bundle.Workgroup, c.BaseBundleDigest)
	if err != nil {
		return err
	}
	data, err := files.Read(root, path, limit)
	if err != nil {
		return err
	}
	if files.Digest(data) != c.BaseBundleDigest {
		return errors.New("composition baseline integrity mismatch")
	}
	var base Bundle
	if err = mail.Decode(data, &base); err != nil {
		return err
	}
	if base.Composition != nil || (base.Version == 1 && bundle.Workgroup != mail.Workgroup) || (base.Version != 1 && base.Workgroup != bundle.Workgroup) {
		return errors.New("composition baseline must be a plain bundle of the same workgroup")
	}
	if err = base.Validate(); err != nil {
		return err
	}
	skill, err := base.SelectedSkill()
	if err != nil {
		return err
	}
	if skill != c.BaseSkill || files.Digest(base.Schema) != c.BaseSchemaDigest {
		return errors.New("composition baseline instructions or schema differ from the declared origin")
	}
	return nil
}

func ComposeSkills(root, group, baseDigest string, scope SkillScope, components []string, limit int64) (Bundle, error) {
	if len(components) == 0 || len(components) > MaxSkillComponents {
		return Bundle{}, errors.New("composition requires a bounded nonempty component list")
	}
	base, err := LoadFor(root, group, baseDigest, limit)
	if err != nil {
		return Bundle{}, err
	}
	// Each candidate is rebuilt from the original domain Skill. Replacing an
	// old component never leaves its instructions hidden in a nested baseline.
	if base.Composition != nil {
		baseDigest = base.Composition.BaseBundleDigest
		base, err = LoadFor(root, group, baseDigest, limit)
		if err != nil {
			return Bundle{}, err
		}
	}
	if base.Composition != nil {
		return Bundle{}, errors.New("nested composition baseline is unsupported")
	}
	selected, err := base.SelectedSkill()
	if err != nil {
		return Bundle{}, err
	}
	c := SkillComposition{Version: ScopedSkillVersion, Scope: scope, BaseBundleDigest: baseDigest, BaseSchemaDigest: files.Digest(base.Schema), BaseSkill: selected, Components: []SkillPin{}}
	for _, digest := range components {
		skill, err := LoadScopedSkill(root, digest, limit)
		if err != nil {
			return Bundle{}, err
		}
		c.Components = append(c.Components, SkillPin{Digest: digest, Content: skill})
	}
	// Ordering is presentation only; no entry overrides an earlier entry.
	rank := map[string]int{"common": 0, "team": 1, "project": 2}
	sort.SliceStable(c.Components, func(i, j int) bool {
		return rank[c.Components[i].Content.Scope.Kind] < rank[c.Components[j].Content.Scope.Kind]
	})
	if err = c.Validate(group, base.Schema); err != nil {
		return Bundle{}, err
	}
	skill := c.RenderSkill()
	bundle := Bundle{Version: BundleVersion, Workgroup: group, Schema: base.Schema, Skill: &skill, Composition: &c}
	data, _ := json.Marshal(bundle)
	if int64(len(data)) > limit {
		return Bundle{}, errors.New("composed bundle exceeds configured artifact limit")
	}
	return bundle, bundle.Validate()
}

func ScopeActivePath(group string, scope SkillScope) (string, error) {
	if !allowedWorkgroup(group) {
		return "", errors.New("unsupported scoped workgroup")
	}
	if err := scope.Validate(); err != nil {
		return "", err
	}
	data, _ := json.Marshal(scope)
	return filepath.ToSlash(filepath.Join("workgroups", "skills", "selections", files.Digest(data), group, "active.json")), nil
}

func ActiveInScope(root, group string, scope *SkillScope, limit int64) (Bundle, string, error) {
	if scope == nil {
		return ActiveFor(root, group, limit)
	}
	path, err := ScopeActivePath(group, *scope)
	if err != nil {
		return Bundle{}, "", err
	}
	data, err := files.Read(root, path, limit)
	if errors.Is(err, os.ErrNotExist) {
		return Bundle{}, "", errors.New("scoped baseline is not initialized; use skills initialize explicitly")
	}
	if err != nil {
		return Bundle{}, "", err
	}
	var selection Selection
	if err = mail.Decode(data, &selection); err != nil {
		return Bundle{}, "", err
	}
	bundle, err := LoadFor(root, group, selection.Digest, limit)
	if err == nil {
		err = ValidateScopeBundle(bundle, *scope)
	}
	return bundle, selection.Digest, err
}

func ValidateScopeBundle(bundle Bundle, scope SkillScope) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if bundle.Composition != nil && bundle.Composition.Scope != scope {
		return errors.New("composed bundle belongs to another application scope")
	}
	return bundle.Validate()
}
