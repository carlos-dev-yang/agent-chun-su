package stagedworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/secrets"
	"chunsu/internal/store"
	"chunsu/internal/webresearch"
)

const webOpenCap = 3

var webAnswerSchema = []byte(`{"type":"object","additionalProperties":false,"required":["status","answer","citations","gaps"],"properties":{"status":{"type":"string","enum":["answered","insufficient_evidence"]},"answer":{"type":"string"},"citations":{"type":"array","items":{"type":"string"}},"gaps":{"type":"array","items":{"type":"string"}}}}`)

type webAnswer struct {
	Status    string   `json:"status"`
	Answer    string   `json:"answer"`
	Citations []string `json:"citations"`
	Gaps      []string `json:"gaps"`
}

func collectWeb(ctx context.Context, in StageInput) (StageOutput, error) {
	settings, err := webresearch.LoadSettings(in.Root)
	if err != nil {
		return StageOutput{}, err
	}
	if !settings.Enabled {
		return StageOutput{}, webresearch.ErrWebDisabled
	}
	if err = authorizeModel(ctx, in, config.RoleCollection); err != nil {
		return StageOutput{}, err
	}
	if err = secrets.BootstrapLinuxStore(in.Root); err != nil {
		return StageOutput{}, err
	}
	vault, err := secrets.Open()
	if err != nil {
		return StageOutput{}, err
	}
	key, err := vault.Get(ctx, webresearch.KeyRef(in.Root))
	if err != nil {
		return StageOutput{}, err
	}
	querySchema := []byte(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string"}}}`)
	prompt, _ := json.Marshal(map[string]any{"task": "Choose one concise public web search query for the original user objective. Do not include private information, credentials, or URLs outside public HTTPS.", "objective": in.Objective})
	queryResult, err := runModel(ctx, in, config.RoleCollection, prompt, querySchema, []byte("Choose a bounded search query only. Do not claim any web result."))
	if err != nil {
		return StageOutput{Receipt: &queryResult}, err
	}
	var choice struct {
		Query string `json:"query"`
	}
	if err = mail.Decode(queryResult.Final, &choice); err != nil {
		return StageOutput{Receipt: &queryResult}, err
	}
	if err = webresearch.ValidateQuery(choice.Query); err != nil {
		return StageOutput{Receipt: &queryResult}, err
	}
	if err = webresearch.ReserveSearch(ctx, in.Root, in.Config.Limits.LockWaitSeconds); err != nil {
		return StageOutput{Receipt: &queryResult}, err
	}
	search, err := webresearch.Search(ctx, choice.Query, key)
	if err != nil {
		return StageOutput{Receipt: &queryResult}, err
	}
	resultData, _ := json.Marshal(queryResult)
	output := StageOutput{Kind: "staged_raw_evidence", Status: store.StepCompleted, Receipt: &queryResult, Extra: map[string][]byte{"collection_query_receipt": resultData}}
	bundle := RawBundle{Version: 1, Workgroup: WebWorkgroup, InputDigest: files.Digest(in.Snapshot), Sources: []RawSource{}, Gaps: []string{}}
	bundle.PinnedMetadata, _ = json.Marshal(map[string]any{"query": choice.Query, "search_provider": search.Provider, "searched_at": search.RetrievedAt, "search_results": search.Results, "public_https_only": true})
	if len(search.Results) == 0 {
		bundle.Gaps = append(bundle.Gaps, "search returned no results")
	}
	openLimit := webOpenCap
	if cap := in.Config.Limits.MaxToolCalls - 1; openLimit > cap {
		openLimit = cap
	}
	if openLimit > in.Config.Limits.MaxMessages {
		openLimit = in.Config.Limits.MaxMessages
	}
	if openLimit < 1 {
		bundle.Gaps = append(bundle.Gaps, "configured tool budget allows no page opens")
	}
	selected := []string{}
	if openLimit > 0 && len(search.Results) > 0 {
		selectSchema := []byte(`{"type":"object","additionalProperties":false,"required":["urls"],"properties":{"urls":{"type":"array","items":{"type":"string"}}}}`)
		selectionPrompt, _ := json.Marshal(map[string]any{"task": "Choose the most relevant URLs from the supplied search results only, up to the specified limit. Output each URL once; do not invent URLs.", "objective": in.Objective, "open_limit": openLimit, "results": search.Results})
		selectionResult, e := runModel(ctx, in, config.RoleCollection, selectionPrompt, selectSchema, []byte("Select URLs from the host search result list only."))
		if e != nil {
			return StageOutput{Receipt: &selectionResult, Extra: output.Extra}, e
		}
		var selection struct {
			URLs []string `json:"urls"`
		}
		if e = mail.Decode(selectionResult.Final, &selection); e != nil {
			return StageOutput{Receipt: &selectionResult, Extra: output.Extra}, e
		}
		allowed := map[string]bool{}
		for _, item := range search.Results {
			allowed[item.URL] = true
		}
		seen := map[string]bool{}
		if len(selection.URLs) > openLimit {
			return StageOutput{Receipt: &selectionResult, Extra: output.Extra}, errors.New("web selection exceeded the configured open budget")
		}
		for _, raw := range selection.URLs {
			if !allowed[raw] || seen[raw] || webresearch.ValidateURL(raw) != nil {
				return StageOutput{Receipt: &selectionResult, Extra: output.Extra}, errors.New("web selection included an unknown, duplicate or disallowed URL")
			}
			seen[raw] = true
			selected = append(selected, raw)
		}
		selectionData, _ := json.Marshal(selectionResult)
		output.Extra["collection_selection_receipt"] = selectionData
		output.Receipt = &selectionResult
	}
	for _, raw := range selected {
		page, e := webresearch.Open(ctx, raw)
		if e != nil {
			bundle.Gaps = append(bundle.Gaps, "page unavailable: "+raw+" ("+webresearch.PublicError(e)+")")
			continue
		}
		metadata, _ := json.Marshal(page)
		bundle.Sources = append(bundle.Sources, RawSource{ID: files.Digest([]byte(page.URL)), URI: page.URL, Digest: files.Digest(metadata), CapturedAt: page.RetrievedAt.Format("2006-01-02T15:04:05Z07:00"), Content: sourceContent(metadata), Metadata: metadata, Omissions: []string{}})
		if page.Truncated {
			bundle.Sources[len(bundle.Sources)-1].Omissions = append(bundle.Sources[len(bundle.Sources)-1].Omissions, "page truncated")
		}
	}
	if len(bundle.Sources) == 0 {
		bundle.Gaps = append(bundle.Gaps, "no readable public pages were collected")
	}
	if _, err = validateRawBundle(bundle, in.Config.Limits); err != nil {
		return output, err
	}
	output.Data, err = json.Marshal(bundle)
	if err != nil {
		return output, err
	}
	if int64(len(output.Data)) > in.Config.Limits.MaxArtifactBytes {
		return output, errors.New("web evidence exceeds artifact budget")
	}
	output.Summary = fmt.Sprintf("Collected %d public pages with %d gaps", len(bundle.Sources), len(bundle.Gaps))
	return output, nil
}

func validateWeb(in StageInput, evidence EvidenceBundle, report []byte) (StageOutput, error) {
	schema, err := mail.CompileSchema(webAnswerSchema)
	if err != nil {
		return StageOutput{}, err
	}
	var raw any
	if err = json.Unmarshal(report, &raw); err != nil {
		return StageOutput{}, err
	}
	if err = schema.Validate(raw); err != nil {
		return StageOutput{}, err
	}
	var answer webAnswer
	if err := mail.Decode(report, &answer); err != nil {
		return StageOutput{}, err
	}
	if evidence.Version != 1 || evidence.Workgroup != WebWorkgroup || evidence.InputDigest != files.Digest(in.Snapshot) {
		return StageOutput{}, errors.New("web answer evidence scope changed")
	}
	if answer.Status != "answered" && answer.Status != "insufficient_evidence" {
		return StageOutput{}, errors.New("web answer status is invalid")
	}
	if strings.TrimSpace(answer.Answer) == "" {
		return StageOutput{}, errors.New("web answer is empty")
	}
	if strings.Contains(answer.Answer, "https://") || strings.Contains(answer.Answer, "http://") {
		return StageOutput{}, errors.New("web answer URLs must come from host-validated citations")
	}
	known := map[string]SourceFacts{}
	for _, source := range evidence.Sources {
		if source.SourceID != files.Digest([]byte(source.URI)) || webresearch.ValidateURL(source.URI) != nil || known[source.SourceID].SourceID != "" {
			return StageOutput{}, errors.New("web evidence source is invalid")
		}
		known[source.SourceID] = source
	}
	seen := map[string]bool{}
	for _, id := range answer.Citations {
		source, ok := known[id]
		if !ok || seen[id] || len(source.Facts) == 0 {
			return StageOutput{}, errors.New("web answer cites an unknown or unsupported source")
		}
		seen[id] = true
	}
	if answer.Status == "answered" && len(answer.Citations) == 0 {
		return StageOutput{}, errors.New("answered web result requires citations")
	}
	if len(evidence.Sources) == 0 && answer.Status != "insufficient_evidence" {
		return StageOutput{}, errors.New("web answer claims support with no readable sources")
	}
	if answer.Status == "insufficient_evidence" && len(answer.Gaps) == 0 && len(evidence.Gaps) == 0 {
		return StageOutput{}, errors.New("insufficient evidence must identify a gap")
	}
	var markdown strings.Builder
	markdown.WriteString(answer.Answer)
	if len(answer.Citations) > 0 {
		markdown.WriteString("\n\nSources:\n")
	}
	for _, id := range answer.Citations {
		source := known[id]
		markdown.WriteString("- [" + id[:12] + "](" + source.URI + ")\n")
	}
	allGaps := append(append([]string{}, evidence.Gaps...), answer.Gaps...)
	if len(allGaps) > 0 {
		markdown.WriteString("\nGaps:\n")
		for _, gap := range allGaps {
			markdown.WriteString("- " + gap + "\n")
		}
	}
	ids := append([]string{}, answer.Citations...)
	validated := Validated{Version: 1, Workgroup: WebWorkgroup, Report: report, Markdown: markdown.String(), Status: answer.Status, Gaps: allGaps, SourceIDs: ids}
	data, err := json.Marshal(validated)
	if err != nil {
		return StageOutput{}, err
	}
	if int64(len(data)) > in.Config.Limits.MaxArtifactBytes {
		return StageOutput{}, errors.New("validated web result exceeds artifact budget")
	}
	return StageOutput{Kind: "staged_validated", Data: data, Status: store.StepCompleted, Summary: answer.Status}, nil
}

// WebSourceURL returns the canonical citation URL for an admitted source ID.
func WebSourceURL(evidence EvidenceBundle, id string) (*url.URL, error) {
	for _, source := range evidence.Sources {
		if source.SourceID == id {
			if err := webresearch.ValidateURL(source.URI); err != nil {
				return nil, err
			}
			return url.Parse(source.URI)
		}
	}
	return nil, errors.New("unknown web source")
}
