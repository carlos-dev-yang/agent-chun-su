package workgroup

import (
	"chunsu/internal/config"
	"chunsu/internal/jira"
	"encoding/json"
)

func jiraDefinition() Definition {
	return Definition{ID: "jira-report", SourceKind: "jira_report_input", Tool: JiraToolName, Server: JiraServerName,
		ValidateInput: func(data []byte, limits config.Limits) error { _, err := jira.ParseReportInput(data); return err },
		PrepareInput:  prepareJiraInput}
}

func prepareJiraInput(p *Package, input []byte) (InputPlan, error) {
	parsed, err := jira.ParseReportInput(input)
	if err != nil {
		return InputPlan{}, err
	}
	p.JiraSnapshot = &parsed
	p.Synthetic = parsed.Snapshot.Synthetic
	index, err := jira.BuildSourceIndex(parsed)
	if err != nil {
		return InputPlan{}, err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	return InputPlan{Index: data, AsOf: parsed.AsOfDate, Timezone: parsed.Policy.Timezone}, err
}
