package workgroup

import (
	"chunsu/internal/codereview"
	"chunsu/internal/config"
	"chunsu/internal/store"
	"encoding/json"
)

func codeDefinition() Definition {
	return Definition{ID: codereview.Workgroup, SourceKind: "code_review_snapshot", Tool: "code_source_get", Server: "chunsu_code",
		AuthorizeInput: func(data []byte, limits config.Limits, route config.Executor) error {
			in, err := codereview.Parse(data, limits)
			if err != nil {
				return err
			}
			return codereview.Authorize(in, route)
		},
		ValidateInput: func(data []byte, limits config.Limits) error { _, err := codereview.Parse(data, limits); return err },
		PrepareInput: func(p *Package, data []byte) (InputPlan, error) {
			in, err := codereview.Parse(data, p.Limits)
			if err != nil {
				return InputPlan{}, err
			}
			if err = codereview.Authorize(in, p.Executor); err != nil {
				return InputPlan{}, err
			}
			p.Synthetic = in.Synthetic
			p.Mode = "read-only-review"
			index, err := codereview.Index(in)
			return InputPlan{Index: index, AsOf: in.CapturedAt, Timezone: "UTC"}, err
		},
		SnapshotSources: func(data []byte, limits config.Limits) (map[string]json.RawMessage, error) {
			in, err := codereview.Parse(data, limits)
			if err != nil {
				return nil, err
			}
			out := map[string]json.RawMessage{}
			for _, s := range in.Sources {
				b, e := json.Marshal(s)
				if e != nil {
					return nil, e
				}
				out[s.ID] = b
			}
			return out, nil
		},
		ValidateResult: func(raw, schema, input []byte, limits config.Limits, observed map[string]bool) (ReportPlan, error) {
			plan := ReportPlan{Status: store.Completed}
			in, err := codereview.Parse(input, limits)
			if err == nil {
				plan.Markdown, err = codereview.ValidateReport(raw, schema, in, observed)
			}
			gap := ""
			if err != nil {
				gap = err.Error()
				plan.Status = store.Failed
			}
			plan.Validation = map[string]any{"contract_valid": err == nil, "diagnostic": gap, "semantic_evaluation": "not performed", "repository_commands": "not executed"}
			if err != nil {
				return plan, err
			}
			plan.Sources, err = codereview.Index(in)
			return plan, err
		},
	}
}
