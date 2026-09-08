package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"chunsu/internal/files"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	ReportInputVersion     = 1
	ReportPolicyVersion    = 2
	ReportWindowDays       = 14
	GroupedSelection       = "own_assigned_14d_grouped_v1"
	MetadataAndDescription = "metadata_and_description"
	MetadataOnly           = "metadata_only"
	GroupOverdue           = "overdue"
	GroupDueWithin14Days   = "due_within_14_days"
	GroupUndatedTODO       = "undated_todo"
)

// ReportPolicy is deliberately separate from Policy.  Policy is the immutable
// saved-reader v1 contract; this policy is pinned by a Jira report input.
type ReportPolicy struct {
	Version          int      `json:"version"`
	ConnectionID     string   `json:"connection_id"`
	ProjectKeys      []string `json:"project_keys"`
	BoardIDs         []string `json:"board_ids"`
	SubjectAccountID string   `json:"subject_account_id"`
	SubjectLabel     string   `json:"subject_label"`
	Selection        string   `json:"selection"`
	TodoStatusID     string   `json:"todo_status_id"`
	DueField         string   `json:"due_field"`
	StartField       string   `json:"start_field"`
	Timezone         string   `json:"timezone"`
	WindowDays       int      `json:"window_days"`
	ContentScope     string   `json:"content_scope"`
}

// ReportInput is the versioned, secret-free boundary from the controller to
// the Jira report workgroup.  Snapshot is immutable normalized evidence.
type ReportInput struct {
	Version                int          `json:"version"`
	Policy                 ReportPolicy `json:"policy"`
	ReportPolicyDigest     string       `json:"report_policy_digest"`
	CollectionPolicyDigest string       `json:"collection_policy_digest"`
	Snapshot               Snapshot     `json:"snapshot"`
	AsOfDate               string       `json:"as_of_date"`
	BoardID                string       `json:"board_id"`
	SiteHost               string       `json:"site_host"`
	TodoStatusID           string       `json:"todo_status_id"`
}

type ReportScope struct {
	ProjectKeys  []string `json:"project_keys"`
	BoardID      string   `json:"board_id"`
	Selection    string   `json:"selection"`
	ContentScope string   `json:"content_scope"`
	SubjectLabel string   `json:"subject_label"`
}

type Citation struct {
	ID             string `json:"id"`
	Pointer        string `json:"pointer"`
	CapturedAt     string `json:"captured_at"`
	ResponseDigest string `json:"response_digest"`
}

type SourceIndexItem struct {
	SourceID              string     `json:"source_id"`
	Group                 string     `json:"group"`
	Key                   string     `json:"key"`
	Summary               string     `json:"summary"`
	Status                Status     `json:"status"`
	StartDate             *string    `json:"start_date"`
	StartDateAvailability string     `json:"start_date_availability"`
	DueDate               *string    `json:"due_date"`
	DueDateAvailability   string     `json:"due_date_availability"`
	DescriptionStatus     string     `json:"description_status"`
	Citations             []Citation `json:"citations"`
}

// SourceIndex is a metadata-only discovery aid.  The executor must retrieve
// full issue content through the attempt-scoped gateway before reporting it.
type SourceIndex struct {
	Version                int               `json:"version"`
	Workgroup              string            `json:"workgroup"`
	AsOfDate               string            `json:"as_of_date"`
	Timezone               string            `json:"timezone"`
	SiteHost               string            `json:"site_host"`
	Scope                  ReportScope       `json:"scope"`
	ReportPolicyDigest     string            `json:"report_policy_digest"`
	CollectionPolicyDigest string            `json:"collection_policy_digest"`
	CapturedFrom           string            `json:"captured_from"`
	CapturedThrough        string            `json:"captured_through"`
	RequiredIDs            []string          `json:"required_ids"`
	Sources                []SourceIndexItem `json:"sources"`
}

type ReportGroups struct {
	Overdue         []string `json:"overdue"`
	DueWithin14Days []string `json:"due_within_14_days"`
	UndatedTODO     []string `json:"undated_todo"`
}

type ReportItem struct {
	SourceID  string   `json:"source_id"`
	Key       string   `json:"key"`
	Summary   string   `json:"summary"`
	Status    Status   `json:"status"`
	StartDate *string  `json:"start_date"`
	DueDate   *string  `json:"due_date"`
	Note      string   `json:"note"`
	Citations []string `json:"citations"`
}

type Report struct {
	Version     int          `json:"version"`
	AsOfDate    string       `json:"as_of_date"`
	Timezone    string       `json:"timezone"`
	Scope       ReportScope  `json:"scope"`
	Summary     string       `json:"summary"`
	Groups      ReportGroups `json:"groups"`
	Items       []ReportItem `json:"items"`
	Gaps        []string     `json:"gaps"`
	Assumptions []string     `json:"assumptions"`
}

type Validation struct {
	Valid             bool     `json:"valid"`
	OperationalStatus string   `json:"operational_status"`
	Checks            []string `json:"checks"`
	Gaps              []string `json:"gaps"`
}

func ParseReportInput(data []byte) (ReportInput, error) {
	var in ReportInput
	if err := decode(data, &in); err != nil {
		return in, err
	}
	return in, in.Validate()
}

func (p ReportPolicy) Validate() error {
	if p.Version != ReportPolicyVersion || !files.ValidID(p.ConnectionID) || strings.TrimSpace(p.SubjectAccountID) == "" || strings.TrimSpace(p.SubjectLabel) == "" {
		return errors.New("Jira report policy requires version 2, connection, subject identity, and subject label")
	}
	if p.Selection != GroupedSelection || p.WindowDays != ReportWindowDays || p.TodoStatusID == "" || p.DueField != "duedate" || !validCustomField(p.StartField) {
		return errors.New("Jira report policy must pin the grouped selection, 14-day window, TODO status, due field, and start field")
	}
	if p.ContentScope != MetadataOnly && p.ContentScope != MetadataAndDescription {
		return errors.New("Jira report policy has an unsupported content scope")
	}
	if p.Timezone == "" || p.Timezone == "Local" {
		return errors.New("Jira report policy requires an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return errors.New("Jira report policy has an invalid timezone")
	}
	for _, values := range [][]string{p.ProjectKeys, p.BoardIDs} {
		if len(values) == 0 || !uniqueNonempty(values) {
			return errors.New("Jira report policy scope IDs must be nonempty and unique")
		}
	}
	for _, id := range p.BoardIDs {
		if !positiveDecimal(id) {
			return errors.New("Jira report policy board IDs must be positive decimals")
		}
	}
	return nil
}

// ReportPolicyDigest is the approval/revocation binding for the report policy.
// It is intentionally distinct from Snapshot.PolicyDigest, which belongs to
// the immutable acquisition policy that produced the snapshot.
func ReportPolicyDigest(p ReportPolicy) string {
	data, _ := json.Marshal(p)
	return files.Digest(data)
}

func (in ReportInput) Validate() error {
	if in.Version != ReportInputVersion {
		return errors.New("unsupported Jira report input version")
	}
	if err := in.Policy.Validate(); err != nil {
		return err
	}
	if _, err := time.Parse(time.DateOnly, in.AsOfDate); err != nil {
		return errors.New("Jira report input requires an ISO date boundary")
	}
	if !positiveDecimal(in.BoardID) || !contains(in.Policy.BoardIDs, in.BoardID) || in.TodoStatusID == "" || in.TodoStatusID != in.Policy.TodoStatusID || !validSiteHost(in.SiteHost) {
		return errors.New("Jira report scope does not match its pinned board, TODO status, or site host")
	}
	if !files.ValidDigest(in.CollectionPolicyDigest) || !files.ValidDigest(in.ReportPolicyDigest) || in.CollectionPolicyDigest != in.Snapshot.PolicyDigest || in.ReportPolicyDigest != ReportPolicyDigest(in.Policy) {
		return errors.New("Jira report input policy digest binding mismatch")
	}
	if in.Snapshot.Version != Version || in.Snapshot.Provider != Provider || in.Snapshot.ConnectionID != in.Policy.ConnectionID || in.Snapshot.Timezone != in.Policy.Timezone || (in.Snapshot.Status != Complete && in.Snapshot.Status != Partial) {
		return errors.New("Jira report snapshot binding mismatch")
	}
	_, err := in.expectedGroups()
	return err
}

func BuildSourceIndex(in ReportInput) (SourceIndex, error) {
	if err := in.Validate(); err != nil {
		return SourceIndex{}, err
	}
	groups, err := in.expectedGroups()
	if err != nil {
		return SourceIndex{}, err
	}
	index := SourceIndex{Version: 1, Workgroup: "jira-report", AsOfDate: in.AsOfDate, Timezone: in.Policy.Timezone, SiteHost: in.SiteHost, Scope: in.scope(), ReportPolicyDigest: in.ReportPolicyDigest, CollectionPolicyDigest: in.CollectionPolicyDigest, CapturedFrom: in.Snapshot.CapturedFrom, CapturedThrough: in.Snapshot.CapturedThrough, RequiredIDs: []string{}, Sources: []SourceIndexItem{}}
	for _, issue := range in.Snapshot.Issues {
		item := SourceIndexItem{SourceID: issue.ID, Group: groups[issue.ID], Key: issue.Key, Summary: issue.Summary, Status: issue.Status, StartDate: optionalDate(issue.StartDate), StartDateAvailability: issue.Availability["start_date"], DueDate: optionalDate(issue.DueDate), DueDateAvailability: issue.Availability["due_date"], DescriptionStatus: issue.Description.Status, Citations: []Citation{}}
		for n, evidence := range issue.Evidence {
			item.Citations = append(item.Citations, Citation{ID: citationID(issue.ID, n), Pointer: evidence.Pointer, CapturedAt: evidence.CapturedAt, ResponseDigest: evidence.Response.Digest})
		}
		index.Sources = append(index.Sources, item)
		index.RequiredIDs = append(index.RequiredIDs, issue.ID)
	}
	sort.Slice(index.Sources, func(i, j int) bool { return index.Sources[i].SourceID < index.Sources[j].SourceID })
	sort.Strings(index.RequiredIDs)
	return index, nil
}

func ValidateReport(data, schema []byte, in ReportInput, observed map[string]bool) (Report, Validation, error) {
	var report Report
	validation := Validation{Checks: []string{}, Gaps: []string{}}
	if err := in.Validate(); err != nil {
		return report, validation, err
	}
	compiled, err := CompileReportSchema(schema)
	if err != nil {
		return report, validation, err
	}
	var raw any
	if err = json.Unmarshal(data, &raw); err != nil {
		return report, validation, err
	}
	if err = compiled.Validate(raw); err != nil {
		return report, validation, fmt.Errorf("result schema: %w", err)
	}
	if err = decode(data, &report); err != nil {
		return report, validation, err
	}
	if report.Version != 1 || report.AsOfDate != in.AsOfDate || report.Timezone != in.Policy.Timezone || !sameScope(report.Scope, in.scope()) {
		return report, validation, errors.New("report changed the pinned Jira scope or time boundary")
	}
	index, err := BuildSourceIndex(in)
	if err != nil {
		return report, validation, err
	}
	sources := map[string]SourceIndexItem{}
	expectedGroups, err := in.expectedGroups()
	if err != nil {
		return report, validation, err
	}
	for _, source := range index.Sources {
		sources[source.SourceID] = source
	}
	groupSources, err := report.groupSources()
	if err != nil {
		return report, validation, err
	}
	if len(groupSources) != len(sources) {
		return report, validation, errors.New("report groups do not cover every selected Jira source exactly once")
	}
	for id, group := range expectedGroups {
		if groupSources[id] != group {
			return report, validation, errors.New("report has an incorrect Jira group membership")
		}
	}
	seenItems := map[string]bool{}
	for _, item := range report.Items {
		if seenItems[item.SourceID] {
			return report, validation, errors.New("duplicate Jira report item source")
		}
		seenItems[item.SourceID] = true
		source, ok := sources[item.SourceID]
		if !ok {
			return report, validation, errors.New("report names a source outside the immutable Jira snapshot")
		}
		if observed == nil || !observed[item.SourceID] {
			return report, validation, errors.New("Jira report source was not retrieved through the scoped gateway")
		}
		if item.Key != source.Key || item.Summary != source.Summary || item.Status != source.Status || !sameDate(item.StartDate, source.StartDate) || !sameDate(item.DueDate, source.DueDate) || !sameStrings(item.Citations, citationIDs(source.Citations)) {
			return report, validation, errors.New("Jira report item changed pinned source facts or citations")
		}
	}
	if len(seenItems) != len(sources) {
		return report, validation, errors.New("every selected Jira source requires one report item")
	}
	if in.Snapshot.Status != Complete || len(in.Snapshot.Gaps) > 0 {
		validation.Gaps = append(validation.Gaps, in.Snapshot.Gaps...)
		if len(report.Gaps) == 0 {
			return report, validation, errors.New("report must disclose Jira acquisition gaps")
		}
	}
	validation.Valid = true
	validation.OperationalStatus = "completed"
	if len(validation.Gaps) > 0 || len(report.Gaps) > 0 {
		validation.OperationalStatus = "partial"
	}
	validation.Checks = append(validation.Checks, "result_schema", "pinned_scope", "exact_group_coverage", "source_fact_fidelity", "gateway_lookups", "source_citations")
	return report, validation, nil
}

func CompileReportSchema(data []byte) (*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	var inspect func(any) error
	inspect = func(value any) error {
		switch node := value.(type) {
		case map[string]any:
			for key, child := range node {
				if key == "$id" {
					return errors.New("workgroup schemas must not redefine their resource identity")
				}
				if key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" {
					ref, ok := child.(string)
					if !ok || !strings.HasPrefix(ref, "#") {
						return errors.New("schema references must remain inside the pinned document")
					}
				}
				if err := inspect(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range node {
				if err := inspect(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := inspect(document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("jira-report.json", document); err != nil {
		return nil, err
	}
	return compiler.Compile("jira-report.json")
}

// RenderMarkdown uses host-generated evidence links only.  It never renders
// model-provided URLs or source bodies.
func RenderMarkdown(report Report, validation Validation, index SourceIndex, todoStatusID, sourceFile string) []byte {
	var out strings.Builder
	escape := markdownText
	fmt.Fprintf(&out, "```yaml\nas_of_date: %s\ndue_window: %s ~ %s\nadditional_scope: overdue, undated reviewed TODO\ncaptured_from: %s\ncaptured_through: %s\ntimezone: %s\n```\n", escape(report.AsOfDate), escape(report.AsOfDate), escape(addDays(report.AsOfDate, ReportWindowDays)), escape(index.CapturedFrom), escape(index.CapturedThrough), escape(report.Timezone))
	fmt.Fprintf(&out, "\n# Jira 업무 리포트\n\n보드: %s-%s  \n프로젝트: %s  \n담당자: %s  \n수집 원문: %s  \n처리 상태: %s\n", escape(strings.Join(report.Scope.ProjectKeys, ",")), escape(report.Scope.BoardID), escape(strings.Join(report.Scope.ProjectKeys, ", ")), escape(report.Scope.SubjectLabel), evidenceLink(sourceFile, "보존된 수집 근거"), escape(validation.OperationalStatus))
	items := map[string]ReportItem{}
	for _, item := range report.Items {
		items[item.SourceID] = item
	}
	todoCount := 0
	for _, item := range report.Items {
		if item.Status.ID == todoStatusID {
			todoCount++
		}
	}
	fmt.Fprintf(&out, "\n## 업무 현황\n\n- 보고 업무: %d건\n  - %d건: %d일 내 기한\n  - %d건: 기한 경과\n  - %d건: 기한 없는 TODO\n- TODO: %d건\n  - 기한 있음: %d건\n  - 기한 없음: %d건\n  - 위 보고 업무의 부분집합입니다.\n", len(report.Items), len(report.Groups.DueWithin14Days), ReportWindowDays, len(report.Groups.Overdue), len(report.Groups.UndatedTODO), todoCount, todoCount-len(report.Groups.UndatedTODO), len(report.Groups.UndatedTODO))
	renderHighlights(&out, report, items)
	fmt.Fprintf(&out, "\n%s\n", escape(report.Summary))
	renderUpcomingTable(&out, report.AsOfDate, report.Groups.DueWithin14Days, items, index.SiteHost, sourceFile)
	renderOverdueTable(&out, report.AsOfDate, report.Groups.Overdue, items, index.SiteHost, sourceFile)
	renderUndatedTable(&out, report.AsOfDate, report.Groups.UndatedTODO, items, index.SiteHost, sourceFile)
	if len(report.Gaps)+len(validation.Gaps) > 0 {
		out.WriteString("\n## 범위와 한계\n")
		for _, gap := range append(append([]string{}, report.Gaps...), validation.Gaps...) {
			fmt.Fprintf(&out, "- %s\n", escape(gap))
		}
	}
	if len(report.Assumptions) > 0 {
		out.WriteString("\n## 가정\n")
		for _, assumption := range report.Assumptions {
			fmt.Fprintf(&out, "- %s\n", escape(assumption))
		}
	}
	return []byte(out.String())
}

func renderHighlights(out *strings.Builder, report Report, items map[string]ReportItem) {
	asOf, _ := time.Parse(time.DateOnly, report.AsOfDate)
	weekStart := asOf.AddDate(0, 0, -int((int(asOf.Weekday())+6)%7))
	weekEnd := weekStart.AddDate(0, 0, 6)
	today, thisWeek := []string{}, []string{}
	var nextStart *time.Time
	for _, item := range items {
		if item.DueDate != nil {
			if due, err := time.Parse(time.DateOnly, *item.DueDate); err == nil {
				if due.Equal(asOf) {
					today = append(today, item.Key)
				}
				if !due.Before(weekStart) && !due.After(weekEnd) {
					thisWeek = append(thisWeek, item.Key)
				}
			}
		}
		if item.StartDate != nil {
			if start, err := time.Parse(time.DateOnly, *item.StartDate); err == nil && start.After(asOf) && (nextStart == nil || start.Before(*nextStart)) {
				copy := start
				nextStart = &copy
			}
		}
	}
	sort.Strings(today)
	sort.Strings(thisWeek)
	weekLabel := "이번 주 기한"
	if len(today) > 0 {
		weekLabel += " (오늘 포함)"
	}
	fmt.Fprintf(out, "\n## 하이라이트\n\n- 오늘 기한: %s\n- %s: %s\n- 기한 경과: %d건\n- 기한 미등록 TODO: %d건\n", compactKeys(today), weekLabel, compactKeys(thisWeek), len(report.Groups.Overdue), len(report.Groups.UndatedTODO))
	if nextStart != nil {
		value := nextStart.Format(time.DateOnly)
		fmt.Fprintf(out, "- 다음 착수일: %s\n", displayDate(report.AsOfDate, &value))
	} else {
		out.WriteString("- 다음 착수일: 기준일 이후 등록된 시작일 없음\n")
	}
}

func renderUpcomingTable(out *strings.Builder, asOf string, ids []string, items map[string]ReportItem, host, sourceFile string) {
	fmt.Fprintf(out, "\n## 14일 내 기한 (%d)\n\n| 업무 | 상태 | 시작 → 기한 | 다음 확인 | 근거 |\n| --- | --- | --- | --- | --- |\n", len(ids))
	for _, id := range ids {
		item := items[id]
		fmt.Fprintf(out, "| %s | %s | %s → %s | %s | %s |\n", jiraLink(host, item.Key, item.Summary), markdownText(item.Status.Name), displayDate(asOf, item.StartDate), displayDate(asOf, item.DueDate), markdownText(item.Note), citationLinks(sourceFile, item.Citations))
	}
}

func renderOverdueTable(out *strings.Builder, asOf string, ids []string, items map[string]ReportItem, host, sourceFile string) {
	fmt.Fprintf(out, "\n## 기한 경과 (%d)\n\n| 업무 | 상태 | 시작 → 기한 | 경과 | 다음 확인 | 근거 |\n| --- | --- | --- | --- | --- | --- |\n", len(ids))
	for _, id := range ids {
		item := items[id]
		fmt.Fprintf(out, "| %s | %s | %s → %s | %d일 | %s | %s |\n", jiraLink(host, item.Key, item.Summary), markdownText(item.Status.Name), displayDate(asOf, item.StartDate), displayDate(asOf, item.DueDate), overdueDays(asOf, item.DueDate), markdownText(item.Note), citationLinks(sourceFile, item.Citations))
	}
}

func renderUndatedTable(out *strings.Builder, asOf string, ids []string, items map[string]ReportItem, host, sourceFile string) {
	fmt.Fprintf(out, "\n## 기한 없는 TODO (%d)\n\n| 업무 | 시작 | 다음 확인 | 근거 |\n| --- | --- | --- | --- |\n", len(ids))
	for _, id := range ids {
		item := items[id]
		fmt.Fprintf(out, "| %s | %s | %s | %s |\n", jiraLink(host, item.Key, item.Summary), displayDate(asOf, item.StartDate), markdownText(item.Note), citationLinks(sourceFile, item.Citations))
	}
}

func jiraLink(host, key, summary string) string {
	return fmt.Sprintf("[%s · %s](https://%s/browse/%s)", markdownText(trimTitle(summary)), markdownText(key), host, url.PathEscape(key))
}
func evidenceLink(file, label string) string {
	if file == "" {
		return markdownText(label)
	}
	return fmt.Sprintf("[%s](%s)", markdownText(label), url.PathEscape(file))
}
func citationLinks(file string, ids []string) string {
	if len(ids) == 0 {
		return "—"
	}
	links := make([]string, 0, len(ids))
	for _, id := range ids {
		links = append(links, evidenceLink(file, id))
	}
	return strings.Join(links, ", ")
}
func compactKeys(keys []string) string {
	if len(keys) == 0 {
		return "없음"
	}
	return markdownText(strings.Join(keys, ", "))
}
func overdueDays(asOf string, due *string) int {
	if due == nil {
		return 0
	}
	a, e1 := time.Parse(time.DateOnly, asOf)
	d, e2 := time.Parse(time.DateOnly, *due)
	if e1 != nil || e2 != nil || !d.Before(a) {
		return 0
	}
	return int(a.Sub(d).Hours() / 24)
}
func trimTitle(value string) string {
	value = strings.TrimSpace(value)
	const limit = 72
	if value == "" {
		return "제목 없음"
	}
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}

func (in ReportInput) scope() ReportScope {
	keys := append([]string{}, in.Policy.ProjectKeys...)
	sort.Strings(keys)
	return ReportScope{ProjectKeys: keys, BoardID: in.BoardID, Selection: in.Policy.Selection, ContentScope: in.Policy.ContentScope, SubjectLabel: in.Policy.SubjectLabel}
}

func (in ReportInput) expectedGroups() (map[string]string, error) {
	asOf, _ := time.Parse(time.DateOnly, in.AsOfDate)
	end := asOf.AddDate(0, 0, in.Policy.WindowDays)
	groups, seen := map[string]string{}, map[string]bool{}
	for _, issue := range in.Snapshot.Issues {
		if seen[issue.ID] || !strings.HasPrefix(issue.ID, in.Policy.ConnectionID+":") || !contains(in.Policy.ProjectKeys, issue.ProjectKey) || issue.Assignee == nil || issue.Assignee.ID != in.Policy.SubjectAccountID || !issue.AssignedToSubject || strings.EqualFold(issue.Status.CategoryKey, "done") || issue.Key == "" || issue.Summary == "" || len(issue.Evidence) == 0 {
			return nil, errors.New("Jira snapshot issue is outside the pinned report scope")
		}
		if issue.StartDate != "" {
			if _, err := time.Parse(time.DateOnly, issue.StartDate); err != nil || issue.Availability["start_date"] != Available {
				return nil, errors.New("Jira snapshot has an invalid configured start date")
			}
		} else if issue.Availability["start_date"] == Available {
			return nil, errors.New("Jira snapshot start-date availability does not match its value")
		}
		seen[issue.ID] = true
		if issue.DueDate == "" {
			if issue.Availability["due_date"] != Empty || issue.Status.ID != in.TodoStatusID {
				return nil, errors.New("Jira snapshot has an unclassifiable undated issue")
			}
			groups[issue.ID] = GroupUndatedTODO
			continue
		}
		due, err := time.Parse(time.DateOnly, issue.DueDate)
		if err != nil || issue.Availability["due_date"] != Available {
			return nil, errors.New("Jira snapshot has an invalid due date")
		}
		switch {
		case due.Before(asOf):
			groups[issue.ID] = GroupOverdue
		case !due.After(end):
			groups[issue.ID] = GroupDueWithin14Days
		default:
			return nil, errors.New("Jira snapshot issue is outside the pinned report date window")
		}
	}
	return groups, nil
}

func (r Report) groupSources() (map[string]string, error) {
	all := map[string]string{}
	for _, pair := range []struct {
		group string
		ids   []string
	}{{GroupOverdue, r.Groups.Overdue}, {GroupDueWithin14Days, r.Groups.DueWithin14Days}, {GroupUndatedTODO, r.Groups.UndatedTODO}} {
		for _, id := range pair.ids {
			if id == "" || all[id] != "" {
				return nil, errors.New("Jira report group sources must be nonempty and disjoint")
			}
			all[id] = pair.group
		}
	}
	return all, nil
}

func citationID(source string, n int) string { return source + "#evidence-" + strconv.Itoa(n+1) }
func citationIDs(values []Citation) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.ID)
	}
	return out
}
func optionalDate(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
func sameDate(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func sameScope(a, b ReportScope) bool {
	return a.BoardID == b.BoardID && a.Selection == b.Selection && a.ContentScope == b.ContentScope && a.SubjectLabel == b.SubjectLabel && sameStrings(a.ProjectKeys, b.ProjectKeys)
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa, bb := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
func uniqueNonempty(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
func positiveDecimal(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}
func validCustomField(value string) bool {
	return strings.HasPrefix(value, "customfield_") && strings.Trim(value[len("customfield_"):], "0123456789") == "" && value != "customfield_"
}
func validSiteHost(value string) bool {
	if value == "" || value != strings.ToLower(value) || strings.ContainsAny(value, "/?#@") {
		return false
	}
	parsed, err := url.Parse("https://" + value)
	return err == nil && parsed.Host == value && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}
func addDays(value string, days int) string {
	at, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return value
	}
	return at.AddDate(0, 0, days).Format(time.DateOnly)
}
func displayDate(asOf string, value *string) string {
	if value == nil {
		return "—"
	}
	at, err := time.Parse(time.DateOnly, *value)
	if err != nil {
		return *value
	}
	base, baseErr := time.Parse(time.DateOnly, asOf)
	if baseErr == nil && base.Year() != at.Year() {
		return at.Format("2006/01/02")
	}
	return at.Format("01/02")
}
func markdownText(value string) string {
	var safe strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			fmt.Fprintf(&safe, "\\u%04x", r)
		} else {
			safe.WriteRune(r)
		}
	}
	value = html.EscapeString(safe.String())
	return strings.NewReplacer("\\", "\\\\", "!", "\\!", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "#", "\\#", "|", "\\|", ">", "\\>").Replace(value)
}
