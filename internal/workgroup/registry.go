package workgroup

import (
	"errors"
	"sort"

	"chunsu/internal/config"
	"chunsu/internal/mail"
)

const MailToolName = "mail_source_get"
const JiraToolName = "jira_issue_get"
const MailServerName = "chunsu_mail"
const JiraServerName = "chunsu_jira"

type InputPlan struct {
	Index    []byte
	AsOf     string
	Timezone string
}

// Definition describes a compiled module. Data/configuration may select a
// module, but cannot register executable code or grant tools at runtime.
type Definition struct {
	ID            string                                    `json:"id"`
	SourceKind    string                                    `json:"source_kind"`
	Tool          string                                    `json:"tool"`
	Server        string                                    `json:"server"`
	ValidateInput func([]byte, config.Limits) error         `json:"-"`
	PrepareInput  func(*Package, []byte) (InputPlan, error) `json:"-"`
}

var definitions = map[string]Definition{
	mail.Workgroup: mailDefinition(),
	"jira-report":  jiraDefinition(),
}

func Lookup(id string) (Definition, error) {
	definition, ok := definitions[id]
	if !ok {
		return Definition{}, errors.New("unsupported workgroup")
	}
	return definition, nil
}

func Definitions() []Definition {
	ids := []string{}
	for id := range definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []Definition{}
	for _, id := range ids {
		out = append(out, definitions[id])
	}
	return out
}

func ValidateInput(id string, data []byte, limits config.Limits) error {
	definition, err := Lookup(id)
	if err != nil {
		return err
	}
	return definition.ValidateInput(data, limits)
}
