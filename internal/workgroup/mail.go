package workgroup

import (
	"chunsu/internal/config"
	"chunsu/internal/mail"
	"encoding/json"
)

func mailDefinition() Definition {
	return Definition{ID: mail.Workgroup, SourceKind: "mail_snapshot", Tool: MailToolName, Server: MailServerName,
		ValidateInput: func(data []byte, limits config.Limits) error { _, err := mail.ParseSnapshot(data, limits); return err },
		PrepareInput:  prepareMailInput}
}

func prepareMailInput(p *Package, input []byte) (InputPlan, error) {
	snapshot, err := mail.ParseSnapshot(input, p.Limits)
	if err != nil {
		return InputPlan{}, err
	}
	p.Snapshot = snapshot
	p.Synthetic = snapshot.Synthetic
	type indexMessage struct {
		ID            string `json:"id"`
		ThreadID      string `json:"thread_id"`
		Scope         string `json:"scope"`
		ReceivedAt    string `json:"received_at"`
		Subject       string `json:"subject"`
		Channel       string `json:"channel"`
		ContentStatus string `json:"content_status"`
	}
	index := []indexMessage{}
	for _, m := range snapshot.Messages {
		index = append(index, indexMessage{m.ID, m.ThreadID, m.Scope, m.ReceivedAt, m.Subject, m.Channel, m.ContentStatus})
	}
	data, err := json.MarshalIndent(map[string]any{"as_of": snapshot.AsOf, "timezone": snapshot.Timezone, "synthetic": snapshot.Synthetic, "collection": snapshot.Collection, "messages": index, "prior_interpretations": snapshot.PriorInterpretations}, "", "  ")
	return InputPlan{Index: data, AsOf: snapshot.AsOf, Timezone: snapshot.Timezone}, err
}
