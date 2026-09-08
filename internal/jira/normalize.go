package jira

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

const (
	MaxRichTextDepth = 32
	ADFVersion       = 1
)

type object map[string]json.RawMessage

func obj(raw json.RawMessage) object { var v object; _ = json.Unmarshal(raw, &v); return v }
func str(raw json.RawMessage) string {
	var v string
	if json.Unmarshal(raw, &v) == nil {
		return v
	}
	return ""
}
func identifier(raw json.RawMessage) string {
	if v := str(raw); v != "" {
		return v
	}
	var v json.Number
	if json.Unmarshal(raw, &v) == nil {
		if _, err := strconv.ParseUint(v.String(), 10, 64); err == nil {
			return v.String()
		}
	}
	return ""
}
func presence(raw json.RawMessage) string {
	if len(raw) == 0 {
		return Missing
	}
	if string(raw) == "null" || string(raw) == `""` {
		return Empty
	}
	return Available
}
func stringPresence(raw json.RawMessage) string {
	state := presence(raw)
	if state != Available {
		return state
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return Invalid
	}
	return Available
}
func person(raw json.RawMessage, mapping Mapping) *Identity {
	v := obj(raw)
	id := identifier(v[mapping.IdentityField])
	if id == "" {
		return nil
	}
	return &Identity{ID: id, DisplayName: str(v["displayName"])}
}

func normalizedText(raw json.RawMessage, limit int64) Text {
	out := Text{Format: "plain", Status: presence(raw)}
	if out.Status != Available {
		return out
	}
	if err := json.Unmarshal(raw, &out.Value); err != nil {
		type node struct {
			Type    string                     `json:"type"`
			Version int                        `json:"version"`
			Text    string                     `json:"text"`
			Attrs   map[string]json.RawMessage `json:"attrs"`
			Content []json.RawMessage          `json:"content"`
		}
		var root node
		if json.Unmarshal(raw, &root) != nil || root.Type != "doc" || root.Version != ADFVersion {
			out.Format = "unknown"
			out.Status = Unsupported
			return out
		}
		out.Format = "adf"
		var b strings.Builder
		var visit func(node, int)
		visit = func(n node, depth int) {
			if int64(b.Len()) > limit {
				out.Status = Truncated
				return
			}
			if depth > MaxRichTextDepth {
				out.Status = Partial
				return
			}
			switch n.Type {
			case "text":
				b.WriteString(n.Text)
			case "hardBreak":
				b.WriteByte('\n')
			case "mention":
				label := str(n.Attrs["text"])
				if label == "" {
					label = str(n.Attrs["id"])
					out.Status = Partial
				}
				b.WriteString(label)
			case "emoji":
				b.WriteString(str(n.Attrs["text"]))
			case "inlineCard", "blockCard":
				b.WriteString(str(n.Attrs["url"]))
			case "doc", "paragraph", "heading", "bulletList", "orderedList", "listItem", "blockquote", "codeBlock", "table", "tableRow", "tableCell", "tableHeader", "rule":
			default:
				out.Status = Partial
			}
			for _, child := range n.Content {
				var next node
				if json.Unmarshal(child, &next) != nil {
					out.Status = Partial
					continue
				}
				visit(next, depth+1)
			}
			switch n.Type {
			case "paragraph", "heading", "listItem", "blockquote", "codeBlock", "tableRow", "rule":
				b.WriteByte('\n')
			}
		}
		visit(root, 0)
		out.Value = strings.TrimSpace(b.String())
	}
	if int64(len(out.Value)) > limit {
		out.Value = out.Value[:limit]
		for !utf8.ValidString(out.Value) && len(out.Value) > 0 {
			out.Value = out.Value[:len(out.Value)-1]
		}
		out.Status = Truncated
	}
	return out
}

func sourceTime(fields object, field string, availability map[string]string) string {
	raw := fields[field]
	availability[field] = presence(raw)
	if availability[field] != Available {
		return ""
	}
	value, err := timestamp(str(raw))
	if err != nil {
		availability[field] = Invalid
		return ""
	}
	return value
}

func sprints(raw json.RawMessage) ([]Sprint, string) {
	out := []Sprint{}
	if p := presence(raw); p != Available {
		return out, p
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return out, Unsupported
	}
	for _, entry := range entries {
		v := obj(entry)
		id := identifier(v["id"])
		if id == "" {
			return out, Unsupported
		}
		s := Sprint{ID: id, Name: str(v["name"]), State: str(v["state"]), BoardID: identifier(v["originBoardId"])}
		for _, pair := range []struct {
			key  string
			dest *string
		}{{"startDate", &s.StartDate}, {"endDate", &s.EndDate}, {"completeDate", &s.CompletedAt}} {
			if presence(v[pair.key]) == Available {
				at, err := timestamp(str(v[pair.key]))
				if err != nil {
					return out, Invalid
				}
				*pair.dest = at
			}
		}
		out = append(out, s)
	}
	return out, Available
}

func seconds(raw json.RawMessage) (*int64, string) {
	if p := presence(raw); p != Available {
		return nil, p
	}
	var value int64
	if json.Unmarshal(raw, &value) != nil || value < 0 {
		return nil, Invalid
	}
	return &value, Available
}

func history(raw json.RawMessage, entriesKey string, mapping Mapping, maxEntries int, byteLimit int64) History {
	h := History{Status: NotCollected, Entries: []ContextEntry{}}
	if presence(raw) != Available {
		return h
	}
	v := obj(raw)
	var entries []json.RawMessage
	if len(v[entriesKey]) == 0 || string(v[entriesKey]) == "null" || json.Unmarshal(v[entriesKey], &entries) != nil {
		h.Status = Partial
		return h
	}
	h.Status = Partial
	var total, start *int
	startOK := len(v["startAt"]) == 0 || (json.Unmarshal(v["startAt"], &start) == nil && start != nil && *start == 0)
	if json.Unmarshal(v["total"], &total) == nil && total != nil && *total >= 0 && startOK && *total == len(entries) {
		h.Status = Complete
	}
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
		h.Status = Partial
	}
	seen := map[string]bool{}
	for _, rawEntry := range entries {
		e := obj(rawEntry)
		id := identifier(e["id"])
		at, err := timestamp(str(e["created"]))
		if id == "" || err != nil || seen[id] {
			h.Status = Partial
			continue
		}
		seen[id] = true
		entry := ContextEntry{ID: id, Author: person(e["author"], mapping), CreatedAt: at}
		if entriesKey == "comments" {
			body := normalizedText(e["body"], byteLimit)
			entry.Body = &body
			if body.Status != Available && body.Status != Empty {
				h.Status = Partial
			}
		} else {
			var changes []json.RawMessage
			if json.Unmarshal(e["items"], &changes) != nil || changes == nil {
				h.Status = Partial
			}
			for _, rawChange := range changes {
				c := obj(rawChange)
				if str(c["field"]) == "" {
					h.Status = Partial
					continue
				}
				entry.Changes = append(entry.Changes, Change{Field: str(c["field"]), FieldID: str(c["fieldId"]), From: identifier(c["from"]), To: identifier(c["to"]), FromText: str(c["fromString"]), ToText: str(c["toString"])})
			}
		}
		h.Entries = append(h.Entries, entry)
	}
	return h
}

type NormalizedPage struct {
	Issues       []Issue
	Dispositions []Disposition
	Gaps         []string
	Rows         int
}

// NormalizePage reads only preserved response bytes and the pinned policy.
// It does not fetch links, infer workload, or complete absent history.
func NormalizePage(page Page, policy Policy, response Reference, remaining int, limits config.Limits) (NormalizedPage, error) {
	out := NormalizedPage{Issues: []Issue{}, Dispositions: []Disposition{}, Gaps: []string{}}
	if err := policy.Validate(); err != nil {
		return out, err
	}
	if int64(len(page.Body)) > limits.MaxArtifactBytes || response.Digest != files.Digest(page.Body) || response.Bytes != int64(len(page.Body)) {
		return out, errors.New("Jira response binding/size mismatch")
	}
	at, err := timestamp(page.CapturedAt)
	if err != nil {
		return out, err
	}
	var body struct {
		Issues []json.RawMessage `json:"issues"`
	}
	if json.Unmarshal(page.Body, &body) != nil || body.Issues == nil {
		return out, errors.New("Jira response lacks an issues array")
	}
	if remaining < 0 {
		return out, errors.New("invalid remaining issue budget")
	}
	if len(body.Issues) > remaining {
		body.Issues = body.Issues[:remaining]
		out.Gaps = append(out.Gaps, "issue_budget_exhausted")
	}
	for index, raw := range body.Issues {
		out.Rows++
		provenance := Provenance{Response: response, Pointer: issuePointerPrefix + strconv.Itoa(index), CapturedAt: at}
		v := obj(raw)
		fields := obj(v["fields"])
		id := identifier(v["id"])
		key := str(v["key"])
		project := obj(fields["project"])
		d := Disposition{SourceID: sourceID(policy.ConnectionID, id), Evidence: provenance}
		if id == "" || key == "" || str(project["key"]) == "" {
			d.SourceID = ""
			d.Outcome = "unresolved"
			d.Reason = "missing_issue_identity"
			out.Dispositions = append(out.Dispositions, d)
			out.Gaps = append(out.Gaps, d.Reason)
			continue
		}
		if !contains(policy.ProjectKeys, str(project["key"])) {
			d.Outcome = "outside_scope"
			d.Reason = "project_not_allowed"
			out.Dispositions = append(out.Dispositions, d)
			out.Gaps = append(out.Gaps, d.Reason)
			continue
		}
		i := Issue{ID: d.SourceID, Key: key, ProjectID: identifier(project["id"]), ProjectKey: str(project["key"]), Summary: str(fields["summary"]), Availability: map[string]string{}, Limitations: []string{}, Evidence: []Provenance{provenance}, Relationships: []Relationship{}}
		i.Availability["summary"] = stringPresence(fields["summary"])
		i.Assignee = person(fields["assignee"], policy.Mapping)
		i.Availability["assignee"] = presence(fields["assignee"])
		if i.Assignee != nil {
			i.AssignedToSubject = i.Assignee.ID == policy.Subject.ID
		} else if i.Availability["assignee"] == Available {
			i.Availability["assignee"] = Invalid
		}
		i.Sprints = []Sprint{}
		i.Availability["sprints"] = "not_mapped"
		if policy.Mapping.SprintField != "" {
			i.Sprints, i.Availability["sprints"] = sprints(fields[policy.Mapping.SprintField])
		}
		for _, sprint := range i.Sprints {
			if contains(policy.SprintIDs, sprint.ID) {
				i.InSelectedSprint = true
			}
		}
		if !i.AssignedToSubject && !(policy.Selection == "assigned_or_sprint" && i.InSelectedSprint) {
			d.Outcome = "outside_selection"
			d.Reason = "not_assigned_or_in_selected_sprint"
			if i.Availability["assignee"] == Missing || i.Availability["assignee"] == Invalid || (policy.Selection == "assigned_or_sprint" && i.Availability["sprints"] != Available && i.Availability["sprints"] != Empty) {
				d.Outcome = "unresolved"
				d.Reason = "selection_fields_unavailable"
				out.Gaps = append(out.Gaps, d.Reason)
			}
			out.Dispositions = append(out.Dispositions, d)
			continue
		}
		i.Description = normalizedText(fields["description"], limits.MaxSourceBytes)
		i.Availability["description"] = i.Description.Status
		status := obj(fields["status"])
		i.Status = Status{ID: identifier(status["id"]), Name: str(status["name"]), CategoryKey: str(obj(status["statusCategory"])["key"])}
		i.Availability["status"] = presence(fields["status"])
		if i.Availability["status"] == Available && (i.Status.ID == "" || i.Status.Name == "") {
			i.Availability["status"] = Invalid
		}
		i.CreatedAt = sourceTime(fields, "created", i.Availability)
		i.UpdatedAt = sourceTime(fields, "updated", i.Availability)
		i.ResolvedAt = sourceTime(fields, "resolutiondate", i.Availability)
		i.Resolution = str(obj(fields["resolution"])["name"])
		i.Availability["resolution"] = presence(fields["resolution"])
		if i.Availability["resolution"] == Available && i.Resolution == "" {
			i.Availability["resolution"] = Invalid
		}
		i.DueDate = str(fields["duedate"])
		i.Availability["due_date"] = presence(fields["duedate"])
		if i.Availability["due_date"] == Available {
			if _, e := time.Parse(time.DateOnly, i.DueDate); e != nil {
				i.Availability["due_date"] = Invalid
				i.DueDate = ""
			}
		}
		if policy.Mapping.StartField != "" {
			i.StartDate = str(fields[policy.Mapping.StartField])
			i.Availability["start_date"] = presence(fields[policy.Mapping.StartField])
			if i.Availability["start_date"] == Available {
				if _, e := time.Parse(time.DateOnly, i.StartDate); e != nil {
					i.Availability["start_date"] = Invalid
					i.StartDate = ""
				}
			}
		}
		tracking := obj(fields["timetracking"])
		i.Availability["timetracking"] = presence(fields["timetracking"])
		if i.Availability["timetracking"] == Available && tracking == nil {
			i.Availability["timetracking"] = Invalid
		}
		i.Estimates.OriginalSeconds, i.Availability["original_seconds"] = seconds(tracking["originalEstimateSeconds"])
		i.Estimates.RemainingSeconds, i.Availability["remaining_seconds"] = seconds(tracking["remainingEstimateSeconds"])
		i.Estimates.SpentSeconds, i.Availability["spent_seconds"] = seconds(tracking["timeSpentSeconds"])
		i.Availability["story_points"] = "not_mapped"
		if field := policy.Mapping.StoryPointsField; field != "" {
			i.Availability["story_points"] = presence(fields[field])
			var points float64
			if i.Availability["story_points"] == Available {
				if json.Unmarshal(fields[field], &points) != nil || points < 0 || math.IsNaN(points) || math.IsInf(points, 0) {
					i.Availability["story_points"] = Invalid
				} else {
					i.Estimates.StoryPoints = &points
				}
			}
		}
		i.Comments = history(fields["comment"], "comments", policy.Mapping, policy.MaxContextEntries, limits.MaxSourceBytes)
		i.Changelog = history(v["changelog"], "histories", policy.Mapping, policy.MaxContextEntries, limits.MaxSourceBytes)
		i.Availability["comments"] = i.Comments.Status
		i.Availability["changelog"] = i.Changelog.Status
		i.Availability["relationships"] = presence(fields["issuelinks"])
		var links []json.RawMessage
		if i.Availability["relationships"] == Available && json.Unmarshal(fields["issuelinks"], &links) != nil {
			i.Availability["relationships"] = Invalid
		}
		for _, link := range links {
			l := obj(link)
			for _, direction := range []string{"inwardIssue", "outwardIssue"} {
				if other := obj(l[direction]); len(other) > 0 {
					otherID := identifier(other["id"])
					if otherID == "" {
						i.Availability["relationships"] = Partial
						continue
					}
					i.Relationships = append(i.Relationships, Relationship{Type: str(obj(l["type"])["name"]), Direction: direction, IssueID: sourceID(policy.ConnectionID, otherID), IssueKey: str(other["key"]), Context: NotCollected})
				}
			}
		}
		captured, _ := time.Parse(time.RFC3339Nano, at)
		if updated, e := time.Parse(time.RFC3339Nano, i.UpdatedAt); e == nil && updated.After(captured) {
			i.Limitations = append(i.Limitations, "source_update_after_capture")
		}
		for field, state := range i.Availability {
			if state == Invalid || state == Unsupported || state == Truncated || state == Partial {
				i.Limitations = append(i.Limitations, field+":"+state)
			}
		}
		sort.Strings(i.Limitations)
		if len(i.Limitations) > 0 {
			out.Gaps = append(out.Gaps, "incomplete_issue_evidence:"+i.ID)
		}
		d.Outcome = "included"
		d.Reason = "within_pinned_selection"
		out.Dispositions = append(out.Dispositions, d)
		out.Issues = append(out.Issues, i)
	}
	return out, nil
}
