package workgroup

import (
	"errors"
	"strings"

	"chunsu/internal/mail"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

func (b Bundle) SelectedSkill() (Skill, error) {
	if b.Version == 1 {
		if err := b.Validate(); err != nil {
			return Skill{}, err
		}
		return legacyMailSkill(b.Guide)
	}
	if b.Version != BundleVersion || b.Skill == nil {
		return Skill{}, errors.New("workgroup bundle has no selected skill")
	}
	if err := b.Skill.Validate(); err != nil {
		return Skill{}, err
	}
	return *b.Skill, nil
}

func (b *Bundle) SetGuide(data []byte) error {
	if isSkillDocument(data) {
		skill, err := parseSkill(data)
		if err != nil {
			return err
		}
		if b.Version == 1 {
			if skill.Name != mail.Workgroup {
				return errors.New("legacy workgroup bundle must use mail-review")
			}
			b.Version = BundleVersion
			b.Workgroup = skill.Name
			b.Guide = ""
			b.Skill = &skill
			return nil
		}
		if b.Version != BundleVersion || b.Workgroup != skill.Name {
			return errors.New("SKILL.md does not match workgroup")
		}
		b.Skill = &skill
		return nil
	}
	if b.Version == 1 {
		b.Guide = string(data)
		return nil
	}
	if b.Version != BundleVersion {
		return errors.New("unsupported workgroup bundle version")
	}
	skill, err := b.SelectedSkill()
	if err != nil {
		return err
	}
	skill.Markdown = skillDocument(skill.Name, skill.Description, string(data))
	if err := skill.Validate(); err != nil {
		return err
	}
	b.Skill = &skill
	return nil
}

func (s Skill) Validate() error {
	if !allowedWorkgroup(s.Name) || strings.TrimSpace(s.Description) == "" || strings.TrimSpace(s.Markdown) == "" {
		return errors.New("invalid workgroup skill")
	}
	parsed, err := parseSkill([]byte(s.Markdown))
	if err != nil {
		return err
	}
	if parsed.Name != s.Name || parsed.Description != s.Description {
		return errors.New("workgroup skill metadata does not match SKILL.md")
	}
	return nil
}

func parseSkill(data []byte) (Skill, error) {
	markdown := string(data)
	lines := strings.Split(markdown, "\n")
	if len(lines) < 5 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return Skill{}, errors.New("SKILL.md needs YAML front matter")
	}
	values := map[string]string{}
	i := 1
	for ; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || (key != "name" && key != "description") || strings.TrimSpace(value) == "" {
			return Skill{}, errors.New("SKILL.md front matter must define name and description")
		}
		if _, exists := values[key]; exists {
			return Skill{}, errors.New("SKILL.md front matter repeats a field")
		}
		values[key] = strings.TrimSpace(value)
	}
	if i == len(lines) || len(values) != 2 || !allowedWorkgroup(values["name"]) {
		return Skill{}, errors.New("invalid SKILL.md front matter")
	}
	if strings.TrimSpace(strings.Join(lines[i+1:], "\n")) == "" {
		return Skill{}, errors.New("SKILL.md body is empty")
	}
	return Skill{Name: values["name"], Description: values["description"], Markdown: markdown}, nil
}

func isSkillDocument(data []byte) bool {
	lines := strings.SplitN(string(data), "\n", 2)
	return len(lines) > 0 && strings.TrimSuffix(lines[0], "\r") == "---"
}
