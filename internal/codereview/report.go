package codereview

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"chunsu/internal/mail"
)

type Finding struct {
	SourceID  string `json:"source_id"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Evidence  string `json:"evidence"`
}
type Report struct {
	Version           int       `json:"version"`
	Repository        string    `json:"repository"`
	BaseRevision      string    `json:"base_revision"`
	HeadRevision      string    `json:"head_revision"`
	Summary           string    `json:"summary"`
	ReviewedSourceIDs []string  `json:"reviewed_source_ids"`
	Findings          []Finding `json:"findings"`
	Limitations       []string  `json:"limitations"`
}

func ValidateReport(raw, schema []byte, in Input, observed map[string]bool) ([]byte, error) {
	compiled, err := mail.CompileSchema(schema)
	if err != nil {
		return nil, err
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if err = compiled.Validate(value); err != nil {
		return nil, err
	}
	var report Report
	if err = mail.Decode(raw, &report); err != nil {
		return nil, err
	}
	if report.Version != Version || report.Repository != in.Repository || report.BaseRevision != in.BaseRevision || report.HeadRevision != in.HeadRevision || !mail.Nonempty(report.Summary) || report.Findings == nil || report.Limitations == nil {
		return nil, errors.New("review result differs from the pinned repository/revisions or has missing fields")
	}
	sources := map[string]Source{}
	for _, s := range in.Sources {
		sources[s.ID] = s
		if !observed[s.ID] {
			return nil, errors.New("required_source_lookups_missing")
		}
	}
	seen := map[string]bool{}
	for _, id := range report.ReviewedSourceIDs {
		if _, ok := sources[id]; !ok || seen[id] {
			return nil, errors.New("reviewed source is duplicate or outside scope")
		}
		seen[id] = true
	}
	if len(seen) != len(sources) {
		return nil, errors.New("review result omits a selected source")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# Code review — %s\n\n%s\n\nRevisions: `%s` → `%s`\n\nScope: %d explicitly selected committed files. Working-tree changes are excluded.\n\n", in.Repository, report.Summary, in.BaseRevision, in.HeadRevision, len(in.Sources))
	for _, f := range report.Findings {
		s, ok := sources[f.SourceID]
		if !ok || !mail.Nonempty(f.Title) || !mail.Nonempty(f.Detail) || !mail.Nonempty(f.Evidence) {
			return nil, errors.New("finding requires a selected source and concrete evidence")
		}
		content := s.After
		if f.Side == "before" {
			content = s.Before
		} else if f.Side != "after" {
			return nil, errors.New("finding side must be before or after")
		}
		lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
		if content == "" || f.StartLine < 1 || f.EndLine < f.StartLine || f.EndLine > len(lines) {
			return nil, errors.New("finding line range lies outside the selected file")
		}
		if !strings.Contains(strings.Join(lines[f.StartLine-1:f.EndLine], "\n"), f.Evidence) {
			return nil, errors.New("finding evidence is absent from the cited lines")
		}
		switch f.Severity {
		case "critical", "high", "medium", "low":
		default:
			return nil, errors.New("unsupported finding severity")
		}
		fmt.Fprintf(&out, "## [%s] %s\n\n`%s` (%s), lines %d–%d\n\n%s\n\n", f.Severity, f.Title, s.Path, f.Side, f.StartLine, f.EndLine, f.Detail)
	}
	out.WriteString("## Verification and limits\n\nThe host checked the schema, pinned revisions, source lookups, source coverage and quoted line ranges. Semantic correctness has not been independently evaluated. No repository build, test, shell command, edit, commit or external publication was performed by this executor.\n\n")
	for _, limit := range report.Limitations {
		fmt.Fprintf(&out, "- %s\n", limit)
	}
	return []byte(out.String()), nil
}
