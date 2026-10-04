package stagedworkflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"chunsu/internal/codereview"
	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/ops"
)

type RawSource struct {
	ID         string          `json:"id"`
	URI        string          `json:"uri"`
	Digest     string          `json:"digest"`
	CapturedAt string          `json:"captured_at,omitempty"`
	Content    string          `json:"content"`
	Metadata   json.RawMessage `json:"metadata"`
	Omissions  []string        `json:"omissions"`
}

type RawBundle struct {
	Version           int             `json:"version"`
	ProjectionVersion string          `json:"projection_version,omitempty"`
	Workgroup         string          `json:"workgroup"`
	InputDigest       string          `json:"input_digest"`
	PinnedMetadata    json.RawMessage `json:"pinned_metadata"`
	Sources           []RawSource     `json:"sources"`
	Gaps              []string        `json:"gaps"`
}

type Fact struct {
	Field     string `json:"field"`
	Value     string `json:"value"`
	Excerpt   string `json:"excerpt"`
	Side      string `json:"side,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type SourceFacts struct {
	SourceID     string   `json:"source_id"`
	URI          string   `json:"uri"`
	SourceDigest string   `json:"source_digest"`
	Facts        []Fact   `json:"facts"`
	Omissions    []string `json:"omissions"`
}

type EvidenceBundle struct {
	Version           int             `json:"version"`
	ProjectionVersion string          `json:"projection_version,omitempty"`
	Workgroup         string          `json:"workgroup"`
	InputDigest       string          `json:"input_digest"`
	RawBundleDigest   string          `json:"raw_bundle_digest"`
	PinnedMetadata    json.RawMessage `json:"pinned_metadata"`
	Sources           []SourceFacts   `json:"sources"`
	Gaps              []string        `json:"gaps"`
}

func snapshotCollection(group string, input []byte, limits config.Limits) (RawBundle, error) {
	return snapshotCollectionVersion(group, input, limits, OpsSourceIndexVersion)
}

// A retained raw artifact selects its exact historical host projection during
// inspection. New execution always uses the current named projection version.
func snapshotCollectionVersion(group string, input []byte, limits config.Limits, projection string) (RawBundle, error) {
	bundle := RawBundle{Version: 1, Workgroup: group, InputDigest: files.Digest(input), Sources: []RawSource{}, Gaps: []string{}}
	switch group {
	case mail.Workgroup:
		s, err := mail.ParseSnapshot(input, limits)
		if err != nil {
			return RawBundle{}, err
		}
		bundle.PinnedMetadata, _ = json.Marshal(map[string]any{"as_of": s.AsOf, "timezone": s.Timezone, "synthetic": s.Synthetic, "collection": s.Collection, "prior_interpretations": s.PriorInterpretations, "origin": s.Origin})
		bundle.Gaps = append(bundle.Gaps, s.Collection.Errors...)
		for _, m := range s.Messages {
			data, _ := json.Marshal(m)
			omissions := []string{}
			if m.ContentStatus != "complete" {
				omissions = append(omissions, "content "+m.ContentStatus)
			}
			for _, a := range m.Attachments {
				omissions = append(omissions, "attachment "+a.ID+": "+a.Status)
			}
			bundle.Sources = append(bundle.Sources, RawSource{ID: m.ID, URI: "mail:" + m.ID, Digest: files.Digest(data), CapturedAt: m.ReceivedAt, Content: sourceContent(data), Metadata: data, Omissions: omissions})
		}
	case "jira-report":
		in, err := jira.ParseReportInput(input)
		if err != nil {
			return RawBundle{}, err
		}
		index, err := jira.BuildSourceIndex(in)
		if err != nil {
			return RawBundle{}, err
		}
		bundle.PinnedMetadata, _ = json.Marshal(map[string]any{"index": index, "policy": in.Policy, "report_policy_digest": in.ReportPolicyDigest, "collection_policy_digest": in.CollectionPolicyDigest, "as_of_date": in.AsOfDate, "board_id": in.BoardID, "site_host": in.SiteHost, "todo_status_id": in.TodoStatusID, "synthetic": in.Snapshot.Synthetic})
		bundle.Gaps = append(bundle.Gaps, in.Snapshot.Gaps...)
		for _, issue := range in.Snapshot.Issues {
			data, _ := json.Marshal(issue)
			bundle.Sources = append(bundle.Sources, RawSource{ID: issue.ID, URI: "https://" + in.SiteHost + "/browse/" + issue.Key, Digest: files.Digest(data), CapturedAt: in.Snapshot.CapturedThrough, Content: sourceContent(data), Metadata: data, Omissions: append([]string{}, issue.Limitations...)})
		}
	case codereview.Workgroup:
		in, err := codereview.Parse(input, limits)
		if err != nil {
			return RawBundle{}, err
		}
		bundle.PinnedMetadata, _ = codereview.Index(in)
		for _, source := range in.Sources {
			data, _ := json.Marshal(source)
			bundle.Sources = append(bundle.Sources, RawSource{ID: source.ID, URI: source.Path, Digest: files.Digest(data), CapturedAt: in.CapturedAt, Content: sourceContent(data), Metadata: data, Omissions: []string{}})
		}
	case ops.Workgroup:
		if projection != "" && projection != OpsSourceIndexVersion {
			return RawBundle{}, errors.New("unsupported team-ops source projection version")
		}
		bundle.ProjectionVersion = projection
		in, err := ops.ParseInput(input, limits)
		if err != nil {
			return RawBundle{}, err
		}
		work, releases := ops.ProjectStatus(in)
		metadata := in
		metadata.Sources = nil
		index := make([]ops.Source, 0, len(in.Sources))
		for _, source := range in.Sources {
			source.Spans = nil
			index = append(index, source)
		}
		pinned := map[string]any{"input": metadata, "host_work_status": work, "host_release_status": releases}
		if projection == OpsSourceIndexVersion {
			pinned["source_index"] = index
		}
		bundle.PinnedMetadata, _ = json.Marshal(pinned)
		bundle.Gaps = append(bundle.Gaps, in.Gaps...)
		for _, source := range in.Sources {
			data, _ := json.Marshal(source)
			omissions := []string{}
			if source.ContentStatus != "complete" {
				omissions = append(omissions, "content "+source.ContentStatus)
			}
			bundle.Sources = append(bundle.Sources, RawSource{ID: source.ID, URI: source.URI, Digest: files.Digest(data), CapturedAt: source.CapturedAt, Content: sourceContent(data), Metadata: data, Omissions: omissions})
		}
	default:
		return RawBundle{}, errors.New("unsupported staged snapshot workgroup")
	}
	return validateRawBundle(bundle, limits)
}

const OpsSpanProjectionVersion = "ops-span-projection-v1"
const OpsHostRefinementIdentity = "host:refine:" + OpsSpanProjectionVersion
const OpsSourceIndexVersion = "ops-source-index-v2"

// HostRefinementReceipt describes an actual deterministic host projection. It
// is deliberately not an executor.Result and makes no model/effort claim.
type HostRefinementReceipt struct {
	Version           int    `json:"version"`
	Kind              string `json:"kind"`
	Workgroup         string `json:"workgroup"`
	Identity          string `json:"identity"`
	ProjectionVersion string `json:"projection_version"`
	CollectionVersion string `json:"collection_version,omitempty"`
	InputDigest       string `json:"input_digest"`
	RawDigest         string `json:"raw_digest"`
	OutputDigest      string `json:"output_digest"`
}

// ProjectOpsEvidence preserves every admitted span verbatim; unlike mail
// refinement it does not call an AI or assert model inspection.
func ProjectOpsEvidence(raw RawBundle, limits config.Limits) ([]byte, HostRefinementReceipt, error) {
	var receipt HostRefinementReceipt
	if raw.Workgroup != ops.Workgroup {
		return nil, receipt, errors.New("host span projection is limited to team-ops")
	}
	if _, err := validateRawBundle(raw, limits); err != nil {
		return nil, receipt, err
	}
	proposed := make([]SourceFacts, 0, len(raw.Sources))
	for _, source := range raw.Sources {
		var original ops.Source
		if err := mail.Decode(source.Metadata, &original); err != nil {
			return nil, receipt, err
		}
		facts := SourceFacts{SourceID: source.ID, Facts: []Fact{}, Omissions: []string{}}
		for _, span := range original.Spans {
			facts.Facts = append(facts.Facts, Fact{Field: span.ID, Value: span.Text, Excerpt: span.Text})
		}
		proposed = append(proposed, facts)
	}
	data, err := buildEvidence(raw, proposed, limits)
	if err != nil {
		return nil, receipt, err
	}
	rawData, _ := json.Marshal(raw)
	receipt = HostRefinementReceipt{Version: 1, Kind: "host_span_projection", Workgroup: ops.Workgroup, Identity: OpsHostRefinementIdentity, ProjectionVersion: OpsSpanProjectionVersion, CollectionVersion: raw.ProjectionVersion, InputDigest: raw.InputDigest, RawDigest: files.Digest(rawData), OutputDigest: files.Digest(data)}
	return data, receipt, nil
}

func validateRawBundle(bundle RawBundle, limits config.Limits) (RawBundle, error) {
	if bundle.Version != 1 || bundle.Workgroup == "" || !files.ValidDigest(bundle.InputDigest) || len(bundle.Sources) > limits.MaxMessages {
		return RawBundle{}, errors.New("invalid raw evidence envelope")
	}
	if (bundle.Workgroup == ops.Workgroup && bundle.ProjectionVersion != "" && bundle.ProjectionVersion != OpsSourceIndexVersion) || (bundle.Workgroup != ops.Workgroup && bundle.ProjectionVersion != "") {
		return RawBundle{}, errors.New("raw evidence has an unsupported host projection")
	}
	seen := map[string]bool{}
	var total int64
	for _, source := range bundle.Sources {
		if source.ID == "" || seen[source.ID] || !files.ValidDigest(source.Digest) || files.Digest(source.Metadata) != source.Digest || source.Content != sourceContent(source.Metadata) {
			return RawBundle{}, errors.New("raw source identity or digest mismatch")
		}
		seen[source.ID] = true
		total += int64(len(source.Content))
		if total > limits.MaxEvidenceBytes {
			return RawBundle{}, errors.New("raw evidence exceeds configured budget")
		}
	}
	return bundle, nil
}

func validateEvidence(raw RawBundle, data []byte, limits config.Limits) (EvidenceBundle, error) {
	var evidence EvidenceBundle
	if int64(len(data)) > limits.MaxArtifactBytes {
		return evidence, errors.New("refined evidence exceeds artifact budget")
	}
	if err := mail.Decode(data, &evidence); err != nil {
		return evidence, err
	}
	rawData, _ := json.Marshal(raw)
	if evidence.Version != 1 || evidence.ProjectionVersion != raw.ProjectionVersion || evidence.Workgroup != raw.Workgroup || evidence.InputDigest != raw.InputDigest || evidence.RawBundleDigest != files.Digest(rawData) || !samePinnedJSON(evidence.PinnedMetadata, raw.PinnedMetadata) || len(evidence.Sources) != len(raw.Sources) {
		return evidence, errors.New("refined evidence changed pinned source scope or metadata")
	}
	known := map[string]RawSource{}
	for _, source := range raw.Sources {
		known[source.ID] = source
	}
	seen := map[string]bool{}
	for _, source := range evidence.Sources {
		rawSource, ok := known[source.SourceID]
		if !ok || seen[source.SourceID] || source.SourceDigest != rawSource.Digest || source.URI != rawSource.URI {
			return evidence, errors.New("refined evidence names an unknown or duplicate raw source")
		}
		seen[source.SourceID] = true
		for _, fact := range source.Facts {
			if strings.TrimSpace(fact.Field) == "" || strings.TrimSpace(fact.Value) == "" || strings.TrimSpace(fact.Excerpt) == "" || (raw.Workgroup != codereview.Workgroup && !strings.Contains(rawSource.Content, fact.Excerpt)) {
				return evidence, fmt.Errorf("fact excerpt is absent from raw source %s", source.SourceID)
			}
			if raw.Workgroup == codereview.Workgroup {
				if err := validateCodeFact(rawSource, fact); err != nil {
					return evidence, err
				}
			}
		}
	}
	return evidence, nil
}

// JSON RawMessage formatting can change when an envelope is marshaled. Ignore
// whitespace only; source IDs, source digests and the raw bundle digest above
// remain exact byte-bound checks.
func samePinnedJSON(a, b json.RawMessage) bool {
	var left, right bytes.Buffer
	if json.Compact(&left, a) != nil || json.Compact(&right, b) != nil {
		return false
	}
	return bytes.Equal(left.Bytes(), right.Bytes())
}

// sourceContent is a deterministic readable projection of the exact pinned
// JSON record. The digest still covers the complete original record.
func sourceContent(data []byte) string {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	lines := []string{}
	var walk func(string, any)
	walk = func(path string, value any) {
		switch node := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(node))
			for key := range node {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(path+"."+key, node[key])
			}
		case []any:
			for i, child := range node {
				walk(fmt.Sprintf("%s[%d]", path, i), child)
			}
		case string:
			lines = append(lines, path+": "+node)
		case nil:
			lines = append(lines, path+": null")
		default:
			lines = append(lines, fmt.Sprintf("%s: %v", path, node))
		}
	}
	walk("source", value)
	return strings.Join(lines, "\n")
}

func validateCodeFact(source RawSource, fact Fact) error {
	var code codereview.Source
	if err := json.Unmarshal(source.Metadata, &code); err != nil {
		return err
	}
	content := code.After
	if fact.Side == "before" {
		content = code.Before
	} else if fact.Side != "after" {
		return errors.New("code fact needs a before/after side")
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if fact.StartLine < 1 || fact.EndLine < fact.StartLine || fact.EndLine > len(lines) || !strings.Contains(strings.Join(lines[fact.StartLine-1:fact.EndLine], "\n"), fact.Excerpt) {
		return errors.New("code fact excerpt is absent from cited lines")
	}
	return nil
}
