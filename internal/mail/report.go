package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Validation struct {
	Valid              bool     `json:"valid"`
	OperationalStatus  string   `json:"operational_status"`
	Checks             []string `json:"checks"`
	Gaps               []string `json:"gaps"`
	SemanticEvaluation string   `json:"semantic_evaluation"`
}

func ValidateReport(data, schema []byte, s Snapshot, mode string, observed map[string]bool) (Report, Validation, error) {
	var r Report
	v := Validation{Checks: []string{}, Gaps: []string{}, SemanticEvaluation: "not performed"}
	var raw, schemaDoc any
	if err := json.Unmarshal(schema, &schemaDoc); err != nil {
		return r, v, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("report.json", schemaDoc); err != nil {
		return r, v, err
	}
	compiled, err := compiler.Compile("report.json")
	if err != nil {
		return r, v, err
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		return r, v, err
	}
	if err = compiled.Validate(raw); err != nil {
		return r, v, fmt.Errorf("result schema: %w", err)
	}
	if err = Decode(data, &r); err != nil {
		return r, v, err
	}
	v.Checks = append(v.Checks, "result_schema")
	if r.AsOf != s.AsOf || r.Timezone != s.Timezone {
		return r, v, errors.New("report changed the pinned time boundary")
	}
	messages := map[string]Message{}
	items := map[string]Item{}
	dispositions := map[string]Disposition{}
	for _, m := range s.Messages {
		messages[m.ID] = m
	}
	for _, it := range r.Items {
		if _, exists := items[it.ID]; exists {
			return r, v, errors.New("duplicate business item ID")
		}
		items[it.ID] = it
		hasTarget := false
		for _, id := range it.Sources {
			m, exists := messages[id]
			if !exists {
				return r, v, fmt.Errorf("unknown source reference %s", id)
			}
			if observed != nil && !observed[id] {
				return r, v, fmt.Errorf("source %s was never retrieved through the scoped gateway", id)
			}
			hasTarget = hasTarget || m.Scope == Target
		}
		if mode == "changes" && !hasTarget {
			return r, v, errors.New("changes mode cannot report a reference-only item as a new change")
		}
	}
	for _, d := range r.Dispositions {
		if _, exists := messages[d.SourceID]; !exists {
			return r, v, errors.New("disposition names an unknown source")
		}
		if _, exists := dispositions[d.SourceID]; exists {
			return r, v, errors.New("duplicate source disposition")
		}
		dispositions[d.SourceID] = d
		if d.Disposition == "included" || d.Disposition == "merged" {
			if len(d.ItemIDs) == 0 {
				return r, v, errors.New("included/merged source needs a business item")
			}
		} else if len(d.ItemIDs) > 0 {
			return r, v, errors.New("non-item disposition cannot link business items")
		}
		for _, id := range d.ItemIDs {
			it, exists := items[id]
			if !exists || !Contains(it.Sources, d.SourceID) {
				return r, v, errors.New("source/item mapping is inconsistent")
			}
		}
		if d.Disposition == "unresolved" {
			v.Gaps = append(v.Gaps, "unresolved source: "+d.SourceID)
		}
	}
	for _, m := range s.Messages {
		if m.Scope == Target {
			if _, exists := dispositions[m.ID]; !exists {
				return r, v, fmt.Errorf("target source %s has no disposition", m.ID)
			}
			if observed != nil && !observed[m.ID] {
				return r, v, fmt.Errorf("target source %s was not inspected", m.ID)
			}
		}
		if m.ContentStatus != "complete" {
			v.Gaps = append(v.Gaps, "source content "+m.ContentStatus+": "+m.ID)
		}
		for _, a := range m.Attachments {
			v.Gaps = append(v.Gaps, "attachment "+a.Status+": "+m.ID+"/"+a.ID)
		}
	}
	for _, it := range r.Items {
		for _, id := range it.Sources {
			d, ok := dispositions[id]
			if !ok || !Contains(d.ItemIDs, it.ID) {
				return r, v, errors.New("every item source needs a matching disposition")
			}
		}
	}
	if s.Collection.Status != "complete" {
		v.Gaps = append(v.Gaps, s.Collection.Errors...)
	}
	if len(v.Gaps) > 0 && len(r.Limitations) == 0 {
		return r, v, errors.New("report must disclose known acquisition or coverage limitations")
	}
	v.Checks = append(v.Checks, "pinned_time_boundary", "source_existence", "target_coverage", "bidirectional_source_mapping", "known_acquisition_gaps")
	v.Valid = true
	v.OperationalStatus = "completed"
	if len(v.Gaps) > 0 {
		v.OperationalStatus = "partial"
	}
	if len(r.Questions) > 0 {
		v.OperationalStatus = "waiting_input"
	}
	return r, v, nil
}

func Render(r Report, v Validation, synthetic bool) []byte {
	var b strings.Builder
	text := func(s string) string {
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		return strings.NewReplacer("[", "\\[", "]", "\\]", "*", "\\*", "`", "\\`", "#", "\\#").Replace(s)
	}
	fmt.Fprintf(&b, "# Mail review\n\nAs of: %s (%s)  \nOperational status: %s  \nSemantic evaluation: %s\n", text(r.AsOf), text(r.Timezone), v.OperationalStatus, v.SemanticEvaluation)
	if synthetic {
		b.WriteString("\n**Synthetic input — this is not a live-work validation.**\n")
	}
	fmt.Fprintf(&b, "\n%s\n", text(r.Summary))
	for _, it := range r.Items {
		fmt.Fprintf(&b, "\n## %s\n\n- Category: %s\n- Why: %s\n- Importance: %s — %s\n- Requested action: %s\n- History: %s\n- Schedule: %s; %s; %s\n- Completion evidence: %s — %s\n- Sources: %s\n", text(it.Title), text(strings.Join(it.Categories, ", ")), text(it.Reason), text(it.Importance), text(it.ImportanceReason), text(it.RequestedAction), text(it.History), text(it.Schedule.Status), text(it.Schedule.When), text(it.Schedule.Impact), text(it.Completion.State), text(it.Completion.Reason), text(strings.Join(it.Sources, ", ")))
		for _, u := range it.Uncertainties {
			fmt.Fprintf(&b, "- Uncertainty: %s\n", text(u))
		}
	}
	b.WriteString("\n## Source dispositions\n\n")
	for _, d := range r.Dispositions {
		fmt.Fprintf(&b, "- %s: %s — %s\n", text(d.SourceID), text(d.Disposition), text(d.Reason))
	}
	if len(r.Limitations)+len(v.Gaps) > 0 {
		b.WriteString("\n## Limitations\n\n")
		for _, l := range append(append([]string{}, r.Limitations...), v.Gaps...) {
			fmt.Fprintf(&b, "- %s\n", text(l))
		}
	}
	if len(r.Questions) > 0 {
		b.WriteString("\n## Questions\n\n")
		for _, q := range r.Questions {
			fmt.Fprintf(&b, "- %s\n", text(q))
		}
	}
	return []byte(b.String())
}
