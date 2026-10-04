package workgroup

import (
	"encoding/json"

	"chunsu/internal/config"
	"chunsu/internal/ops"
)

func opsDefinition() Definition {
	return Definition{ID: ops.Workgroup, SourceKind: "team_ops_saved_input", Tool: "ops_source_get", Server: "chunsu_ops",
		ValidateInput: func(data []byte, limits config.Limits) error { _, err := ops.ParseInput(data, limits); return err },
		AuthorizeInput: func(data []byte, limits config.Limits, route config.Executor) error {
			in, err := ops.ParseInput(data, limits)
			if err != nil {
				return err
			}
			return ops.AuthorizeInput(in, route)
		},
		PrepareInput: func(p *Package, data []byte) (InputPlan, error) {
			in, err := ops.ParseInput(data, p.Limits)
			if err != nil {
				return InputPlan{}, err
			}
			if err = ops.AuthorizeInput(in, p.Executor); err != nil {
				return InputPlan{}, err
			}
			p.Synthetic, p.Mode = in.Synthetic, "saved-shadow"
			return InputPlan{Index: data, AsOf: in.AsOf, Timezone: in.Timezone}, nil
		},
		SnapshotSources: func(data []byte, limits config.Limits) (map[string]json.RawMessage, error) {
			in, err := ops.ParseInput(data, limits)
			if err != nil {
				return nil, err
			}
			out := map[string]json.RawMessage{}
			for _, source := range in.Sources {
				b, e := json.Marshal(source)
				if e != nil {
					return nil, e
				}
				out[source.ID] = b
			}
			return out, nil
		},
		ValidateResult: func(raw, schema, input []byte, limits config.Limits, observed map[string]bool) (ReportPlan, error) {
			in, err := ops.ParseInput(input, limits)
			if err != nil {
				return ReportPlan{}, err
			}
			report, validation, err := ops.ValidateReport(raw, schema, in, observed)
			if err != nil {
				return ReportPlan{}, err
			}
			return ReportPlan{Markdown: ops.RenderReport(report, validation, in), Sources: input, Status: validation.OperationalStatus, Validation: validation}, nil
		},
	}
}
