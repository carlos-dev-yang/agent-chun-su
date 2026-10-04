package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
)

// SlackSnapshot is an owner-imported saved-input contract, not a live Slack
// cursor or a guarantee that the company channel was continuously observed.
type SlackSnapshot struct {
	Version         int            `json:"version"`
	Provider        string         `json:"provider"`
	Scope           Scope          `json:"scope"`
	Synthetic       bool           `json:"synthetic"`
	WorkspaceID     string         `json:"workspace_id"`
	AllowedChannels []string       `json:"allowed_channels"`
	CapturedFrom    string         `json:"captured_from"`
	CapturedThrough string         `json:"captured_through"`
	Coverage        string         `json:"coverage"`
	Gaps            []string       `json:"gaps"`
	Messages        []SlackMessage `json:"messages"`
}

type SlackMessage struct {
	ID            string `json:"id"`
	EventID       string `json:"event_id"`
	Version       string `json:"version"`
	ChannelID     string `json:"channel_id"`
	ThreadID      string `json:"thread_id"`
	SourceAt      string `json:"source_at"`
	Kind          string `json:"kind"`
	URI           string `json:"uri"`
	Text          string `json:"text"`
	ContentStatus string `json:"content_status"`
}

func makeSpans(sourceID, text string) []Span {
	out := []Span{}
	for index, line := range strings.Split(text, "\n") {
		if !nonempty(line) {
			continue
		}
		key := fmt.Sprintf("%s\x00%d\x00%s", sourceID, index, line)
		out = append(out, Span{ID: "span:" + files.Digest([]byte(key)), Text: line})
	}
	return out
}

func sourceIdentity(provider, logical, version string) string {
	return provider + ":" + files.Digest([]byte(logical+"\x00"+version))
}

func NormalizeObservation(id string, scope Scope, format string, data []byte, limits config.Limits) (Observation, error) {
	observation := Observation{ID: id, Version: Version, Scope: scope, Provider: format, Gaps: []string{}, Sources: []Source{}, Raw: append([]byte{}, data...), InputDigest: files.Digest(data)}
	if !identifier(id) || scope.Validate() != nil || int64(len(data)) > limits.MaxArtifactBytes {
		return observation, errors.New("saved observation identity, scope or byte budget is invalid")
	}
	switch format {
	case "slack":
		var snapshot SlackSnapshot
		if err := decode(data, &snapshot); err != nil {
			return observation, err
		}
		if snapshot.Version != Version || snapshot.Provider != "slack" || snapshot.Scope != scope || !nonempty(snapshot.WorkspaceID) || len(snapshot.AllowedChannels) == 0 || snapshot.Messages == nil {
			return observation, errors.New("Slack saved input needs version 1, matching scope, workspace and allowed channels")
		}
		allowed := map[string]bool{}
		for _, channel := range snapshot.AllowedChannels {
			if !nonempty(channel) || allowed[channel] {
				return observation, errors.New("Slack channel scope is empty or duplicated")
			}
			allowed[channel] = true
		}
		observation.Synthetic, observation.CapturedFrom, observation.CapturedThrough, observation.Coverage, observation.Gaps = snapshot.Synthetic, snapshot.CapturedFrom, snapshot.CapturedThrough, snapshot.Coverage, append([]string{}, snapshot.Gaps...)
		for _, message := range snapshot.Messages {
			if !allowed[message.ChannelID] || !nonempty(message.ID) || !nonempty(message.EventID) || !nonempty(message.Version) || !nonempty(message.ThreadID) || !timestamp(message.SourceAt) || !oneOf(message.Kind, "message", "edited", "deleted") || !oneOf(message.ContentStatus, "complete", "truncated", "unavailable", "deleted") || int64(len(message.Text)) > limits.MaxSourceBytes {
				return observation, errors.New("Slack message has invalid identity, time, content status or channel scope")
			}
			if message.Kind == "deleted" && (message.ContentStatus != "deleted" || message.Text != "") {
				return observation, errors.New("deleted Slack message must be a tombstone without retained text")
			}
			logical := snapshot.WorkspaceID + ":" + message.ChannelID + ":" + message.ID
			sourceID := sourceIdentity(format, logical, message.Version)
			text := message.Text
			if message.Kind == "deleted" {
				text = "Saved source version is a deletion tombstone; current message content is unavailable."
			}
			observation.Sources = append(observation.Sources, Source{ID: sourceID, LogicalID: logical, Kind: format, ChangeKind: message.Kind, ExternalID: message.ID, EventID: message.EventID, Version: message.Version, ThreadID: message.ThreadID, ChannelID: message.ChannelID, URI: message.URI, SourceAt: message.SourceAt, CapturedAt: snapshot.CapturedThrough, ContentStatus: message.ContentStatus, Spans: makeSpans(sourceID, text)})
		}
	case "mail":
		snapshot, err := mail.ParseSnapshot(data, limits)
		if err != nil {
			return observation, err
		}
		observation.Synthetic, observation.CapturedFrom, observation.CapturedThrough, observation.Coverage = snapshot.Synthetic, snapshot.AsOf, snapshot.AsOf, snapshot.Collection.Status
		observation.Gaps = append(observation.Gaps, snapshot.Collection.Errors...)
		for _, message := range snapshot.Messages {
			version := files.Digest(mustJSON(message))
			sourceID := sourceIdentity(format, message.ID, version)
			text := "From: " + message.From + "\nSubject: " + message.Subject + "\n" + message.Body
			observation.Sources = append(observation.Sources, Source{ID: sourceID, LogicalID: message.ID, Kind: format, ChangeKind: "message", ExternalID: message.ID, EventID: message.ID, Version: version, ThreadID: message.ThreadID, ChannelID: message.Channel, URI: "mail:" + message.ID, SourceAt: message.ReceivedAt, CapturedAt: snapshot.AsOf, ContentStatus: message.ContentStatus, Spans: makeSpans(sourceID, text)})
			for _, attachment := range message.Attachments {
				observation.Gaps = append(observation.Gaps, "mail attachment "+attachment.ID+": "+attachment.Status)
			}
		}
	case "jira":
		input, err := jira.ParseReportInput(data)
		if err != nil {
			return observation, err
		}
		snapshot := input.Snapshot
		observation.Synthetic, observation.CapturedFrom, observation.CapturedThrough = snapshot.Synthetic, snapshot.CapturedFrom, snapshot.CapturedThrough
		observation.Coverage, observation.Gaps = snapshot.Status, append([]string{}, snapshot.Gaps...)
		for _, issue := range snapshot.Issues {
			logical := snapshot.ConnectionID + ":" + issue.ID
			text := "Issue: " + issue.Key + "\nSummary: " + issue.Summary + "\nExternal Jira status: " + issue.Status.Name + "\nExternal due date: " + issue.DueDate + "\n" + issue.Description.Value
			if issue.Assignee != nil {
				text += "\nExternal assignee: " + issue.Assignee.ID + " (" + issue.Assignee.DisplayName + ")"
			}
			for _, comment := range issue.Comments.Entries {
				if comment.Body != nil {
					text += "\nComment " + comment.ID + " at " + comment.CreatedAt + ": " + comment.Body.Value
				}
			}
			for _, change := range issue.Changelog.Entries {
				for _, field := range change.Changes {
					text += "\nChange at " + change.CreatedAt + ": " + field.Field + " from " + field.FromText + " to " + field.ToText
				}
			}
			status := "complete"
			if issue.ConflictingVersions || len(issue.Limitations) != 0 || issue.Description.Status != jira.Available {
				status = "truncated"
			}
			// A recovered description/comment may enrich the saved evidence even
			// when Jira updated_at is unchanged. Version the projected content too.
			version := issue.UpdatedAt + "|" + files.Digest(mustJSON(struct {
				Text, Status string
				Limitations  []string
			}{text, status, issue.Limitations}))
			sourceID := sourceIdentity(format, logical, version)
			observation.Gaps = append(observation.Gaps, issue.Limitations...)
			observation.Sources = append(observation.Sources, Source{ID: sourceID, LogicalID: logical, Kind: format, ChangeKind: "issue", ExternalID: issue.Key, EventID: logical, Version: version, ThreadID: logical, ChannelID: issue.ProjectKey, URI: "https://" + input.SiteHost + "/browse/" + issue.Key, SourceAt: normalizedJiraTime(issue.UpdatedAt), CapturedAt: snapshot.CapturedThrough, ContentStatus: status, Spans: makeSpans(sourceID, text)})
		}
	default:
		return observation, errors.New("saved observation format must be slack, mail or jira")
	}
	if observation.Coverage == "complete" && len(observation.Gaps) > 0 {
		observation.Coverage = "partial"
	}
	if len(observation.Sources) > limits.MaxMessages {
		return observation, errors.New("saved observation exceeds the source-count budget")
	}
	return observation, observation.Validate()
}

func (observation Observation) Validate() error {
	if observation.Version != Version || observation.Scope.Validate() != nil || !identifier(observation.ID) || !oneOf(observation.Provider, "mail", "jira", "slack") || !timestamp(observation.CapturedFrom) || !timestamp(observation.CapturedThrough) || !oneOf(observation.Coverage, "complete", "partial", "failed", "not_collected") || observation.Gaps == nil || observation.Sources == nil || !json.Valid(observation.Raw) || files.Digest(observation.Raw) != observation.InputDigest {
		return errors.New("saved observation envelope is invalid")
	}
	from, _ := time.Parse(time.RFC3339Nano, observation.CapturedFrom)
	through, _ := time.Parse(time.RFC3339Nano, observation.CapturedThrough)
	if from.After(through) || observation.Coverage == "complete" && len(observation.Gaps) != 0 || observation.Coverage != "complete" && len(observation.Gaps) == 0 {
		return errors.New("saved observation coverage must preserve gaps and its ordered capture window")
	}
	seen := map[string]bool{}
	for _, source := range observation.Sources {
		if !nonempty(source.ID) || seen[source.ID] || source.Kind != observation.Provider || !nonempty(source.LogicalID) || !timestamp(source.CapturedAt) || source.SourceAt != "" && !timestamp(source.SourceAt) || source.Spans == nil {
			return errors.New("saved observation source is invalid or duplicated")
		}
		seen[source.ID] = true
		if source.SourceAt != "" {
			at, _ := time.Parse(time.RFC3339Nano, source.SourceAt)
			if at.After(through) {
				return errors.New("source timestamp exceeds the saved capture window")
			}
		}
		spanIDs := map[string]bool{}
		for _, span := range source.Spans {
			if !nonempty(span.ID) || !nonempty(span.Text) || spanIDs[span.ID] {
				return errors.New("source span is empty or duplicated")
			}
			spanIDs[span.ID] = true
		}
	}
	return nil
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func normalizedJiraTime(value string) string {
	for _, layout := range []string{time.RFC3339Nano, jira.JiraTimestamp} {
		if at, err := time.Parse(layout, value); err == nil {
			return at.UTC().Format(time.RFC3339Nano)
		}
	}
	return ""
}

func sameSourceVersion(left, right Source) bool {
	left.CapturedAt, right.CapturedAt = "", ""
	left.EventID, right.EventID = "", ""
	return sameJSON(left, right)
}

type Input struct {
	Version        int            `json:"version"`
	Synthetic      bool           `json:"synthetic"`
	AsOf           string         `json:"as_of"`
	Timezone       string         `json:"timezone"`
	Scope          Scope          `json:"scope"`
	LedgerRevision int64          `json:"ledger_revision"`
	LedgerHead     RecordRef      `json:"ledger_head"`
	StateDigest    string         `json:"state_digest"`
	Policy         Policy         `json:"policy"`
	WorkItems      []WorkItem     `json:"work_items"`
	ReleaseCycles  []ReleaseCycle `json:"release_cycles"`
	Incidents      []Incident     `json:"incidents"`
	Sources        []Source       `json:"sources"`
	Gaps           []string       `json:"gaps"`
}

type ReportRequest struct {
	Scope Scope  `json:"scope"`
	AsOf  string `json:"as_of"`
}

func inputStateDigest(in Input) string {
	return files.Digest(mustJSON(struct {
		Scope     Scope          `json:"scope"`
		Revision  int64          `json:"revision"`
		Head      RecordRef      `json:"head"`
		Policy    Policy         `json:"policy"`
		Work      []WorkItem     `json:"work"`
		Release   []ReleaseCycle `json:"release"`
		Incidents []Incident     `json:"incidents"`
	}{in.Scope, in.LedgerRevision, in.LedgerHead, in.Policy, in.WorkItems, in.ReleaseCycles, in.Incidents}))
}

func (s Service) BuildInput(ctx context.Context, scope Scope, asOf string) (Input, error) {
	state, history, err := s.Load(ctx, scope)
	if err != nil {
		return Input{}, err
	}
	if state.Revision == 0 {
		return Input{}, errors.New("OPS scope is not initialized")
	}
	if !timestamp(asOf) {
		return Input{}, errors.New("report as_of must be RFC3339")
	}
	at, _ := time.Parse(time.RFC3339Nano, asOf)
	headAt, _ := time.Parse(time.RFC3339Nano, history[len(history)-1].Event.At)
	if at.Before(headAt) {
		return Input{}, errors.New("report as_of precedes the current ledger; historical replay is not available in this local slice")
	}
	in := Input{Version: Version, Synthetic: state.Settings.Synthetic, Scope: scope, AsOf: asOf, Timezone: state.Settings.Timezone, LedgerRevision: state.Revision, LedgerHead: state.Head, Policy: state.Policy, WorkItems: []WorkItem{}, ReleaseCycles: []ReleaseCycle{}, Incidents: []Incident{}, Sources: []Source{}, Gaps: []string{}}
	for _, item := range state.WorkItems {
		in.WorkItems = append(in.WorkItems, item)
	}
	for _, cycle := range state.ReleaseCycles {
		in.ReleaseCycles = append(in.ReleaseCycles, cycle)
	}
	for _, incident := range state.Incidents {
		in.Incidents = append(in.Incidents, incident)
	}
	sort.Slice(in.WorkItems, func(i, j int) bool { return in.WorkItems[i].ID < in.WorkItems[j].ID })
	sort.Slice(in.ReleaseCycles, func(i, j int) bool { return in.ReleaseCycles[i].ID < in.ReleaseCycles[j].ID })
	sort.Slice(in.Incidents, func(i, j int) bool { return in.Incidents[i].ID < in.Incidents[j].ID })
	for _, item := range in.WorkItems {
		in.Sources = append(in.Sources, domainSource("work", item.ID, item.UpdatedAt, asOf, item))
	}
	for _, cycle := range in.ReleaseCycles {
		in.Sources = append(in.Sources, domainSource("release", cycle.ID, history[len(history)-1].Event.At, asOf, cycle))
	}
	for _, incident := range in.Incidents {
		in.Sources = append(in.Sources, domainSource("incident", incident.ID, incident.UpdatedAt, asOf, incident))
	}
	seen := map[string]bool{}
	observationIDs := make([]string, 0, len(state.Observations))
	for id := range state.Observations {
		observationIDs = append(observationIDs, id)
	}
	sort.Strings(observationIDs)
	for _, id := range observationIDs {
		observation := state.Observations[id]
		in.Synthetic = in.Synthetic && observation.Synthetic
		captured, _ := time.Parse(time.RFC3339Nano, observation.CapturedThrough)
		if captured.After(at) {
			return Input{}, errors.New("saved observation capture is after report as_of")
		}
		for _, gap := range observation.Gaps {
			in.Gaps = append(in.Gaps, observation.ID+": "+gap)
		}
		if state.Policy.SourceFreshnessSeconds > 0 && at.Sub(captured) > time.Duration(state.Policy.SourceFreshnessSeconds)*time.Second {
			in.Gaps = append(in.Gaps, observation.ID+": saved evidence is stale under the selected freshness policy")
		}
		for _, source := range observation.Sources {
			if seen[source.ID] {
				continue
			}
			seen[source.ID] = true
			in.Sources = append(in.Sources, source)
			if source.ContentStatus != "complete" {
				in.Gaps = append(in.Gaps, source.ID+": content "+source.ContentStatus)
			}
		}
	}
	in.StateDigest = inputStateDigest(in)
	data := mustJSON(in)
	return ParseInput(data, s.Config.Limits)
}

func domainSource(kind, id, sourceAt, capturedAt string, value any) Source {
	sourceID := "ops:" + kind + ":" + id
	text := string(mustJSON(value))
	return Source{ID: sourceID, LogicalID: sourceID, Kind: "ops_" + kind, ChangeKind: "confirmed", ExternalID: id, EventID: id, Version: files.Digest([]byte(text)), ThreadID: id, ChannelID: "", URI: sourceID, SourceAt: sourceAt, CapturedAt: capturedAt, ContentStatus: "complete", Spans: makeSpans(sourceID, text)}
}

func ParseInput(data []byte, limits config.Limits) (Input, error) {
	var in Input
	if int64(len(data)) > limits.MaxArtifactBytes {
		return in, errors.New("OPS report input exceeds artifact budget")
	}
	if err := decode(data, &in); err != nil {
		return in, err
	}
	if in.Version != Version || in.Scope.Validate() != nil || !timestamp(in.AsOf) || in.LedgerRevision <= 0 || !files.ValidID(in.LedgerHead.ID) || !files.ValidDigest(in.LedgerHead.Digest) || inputStateDigest(in) != in.StateDigest || in.WorkItems == nil || in.ReleaseCycles == nil || in.Incidents == nil || in.Sources == nil || in.Gaps == nil || len(in.Sources) > limits.MaxMessages {
		return in, errors.New("OPS report input has invalid scope, revision, pinned state, arrays or source count")
	}
	if in.Timezone == "" || in.Timezone == "Local" {
		return in, errors.New("OPS report input needs an explicit timezone")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return in, err
	}
	if err := in.Policy.Validate(); err != nil {
		return in, err
	}
	seen := map[string]bool{}
	var total int64
	for _, source := range in.Sources {
		if !nonempty(source.ID) || seen[source.ID] || source.Spans == nil || !timestamp(source.CapturedAt) || source.SourceAt != "" && !timestamp(source.SourceAt) {
			return in, errors.New("OPS report source identity or time is invalid")
		}
		seen[source.ID] = true
		spanIDs := map[string]bool{}
		for _, span := range source.Spans {
			if !nonempty(span.ID) || !nonempty(span.Text) || spanIDs[span.ID] || int64(len(span.Text)) > limits.MaxSourceBytes {
				return in, errors.New("OPS report span is invalid or too large")
			}
			spanIDs[span.ID] = true
			total += int64(len(span.Text))
		}
	}
	if total > limits.MaxEvidenceBytes {
		return in, errors.New("OPS report evidence exceeds budget")
	}
	return in, nil
}

// No team/Slack disclosure grant exists yet. Saved is provenance, not public
// permission. Explicit synthetic fixtures are the only AI admission path.
func AuthorizeInput(in Input, route config.Executor) error {
	_ = route
	if !in.Synthetic {
		return errors.New("team-ops AI currently accepts explicitly declared synthetic fixtures only; private team/Slack disclosure policy is not configured")
	}
	return nil
}
