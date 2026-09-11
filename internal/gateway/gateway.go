package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/jira"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolName = "mail_source_get"
const JiraToolName = "jira_issue_get"

func ToolForWorkgroup(workgroup string) (string, error) {
	switch workgroup {
	case mail.Workgroup:
		return ToolName, nil
	case "jira-report":
		return JiraToolName, nil
	default:
		return "", errors.New("unsupported gateway workgroup")
	}
}

type Input struct {
	SourceID string `json:"source_id" jsonschema:"Exact source ID from this attempt's source index"`
}
type Output struct {
	Status string            `json:"status"`
	Source *mail.Message     `json:"source,omitempty"`
	Issue  *jiraGatewayIssue `json:"issue,omitempty"`
	Detail string            `json:"detail,omitempty"`
}
type Evidence struct {
	Tool     string `json:"tool"`
	SourceID string `json:"source_id"`
	At       string `json:"at"`
	Result   Output `json:"result"`
}

func EvidenceDir(job, attempt string) string {
	return filepath.Join("runs", job, "attempts", attempt, "lookups")
}

func Serve(ctx context.Context, root, jobID, attemptID string) error {
	if !files.ValidID(jobID) || !files.ValidID(attemptID) {
		return errors.New("invalid gateway scope")
	}
	c, err := config.Load(root)
	if err != nil {
		return err
	}
	s, err := store.OpenReadOnly(ctx, root)
	if err != nil {
		return err
	}
	defer s.Close()
	j, err := s.Job(ctx, jobID)
	if err != nil {
		return err
	}
	artifacts, err := s.Artifacts(ctx, jobID)
	if err != nil {
		return err
	}
	var snapshot mail.Snapshot
	var input []byte
	var manifest workgroup.Package
	for _, art := range artifacts {
		if art.Kind == "package_manifest" && art.AttemptID == attemptID {
			b, e := s.ReadArtifact(art, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
			}
			if e = mail.Decode(b, &manifest); e != nil {
				return e
			}
			break
		}
	}
	if manifest.JobID != jobID || manifest.AttemptID != attemptID {
		return errors.New("gateway has no pinned attempt manifest")
	}
	c.Limits = manifest.Limits
	if err = c.Validate(); err != nil {
		return err
	}
	found := false
	for _, art := range artifacts {
		if art.Kind == "input" && art.Path == j.InputRef {
			if art.Digest != manifest.InputDigest {
				return errors.New("gateway input differs from the pinned manifest")
			}
			b, e := s.ReadArtifact(art, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
			}
			input = b
			if manifest.Workgroup == "jira-report" {
				if _, e = jira.ParseReportInput(b); e != nil {
					return e
				}
				found = true
				break
			}
			snapshot, e = mail.ParseSnapshot(b, c.Limits)
			if e != nil {
				return e
			}
			found = true
			break
		}
	}
	if !found {
		return errors.New("gateway input artifact is missing")
	}
	if manifest.Workgroup == "jira-report" {
		reportInput, e := jira.ParseReportInput(input)
		if e != nil {
			return e
		}
		if e = verifyJiraManifest(manifest, reportInput); e != nil {
			return e
		}
		return serveJira(ctx, root, s, c, j, manifest, input)
	}
	if manifest.Workgroup != mail.Workgroup {
		return errors.New("gateway manifest has an unsupported workgroup")
	}
	sources := map[string]mail.Message{}
	for _, m := range snapshot.Messages {
		sources[m.ID] = m
	}
	var mu sync.Mutex
	var calls, evidenceBytes int64
	entries, readErr := os.ReadDir(filepath.Join(root, EvidenceDir(jobID, attemptID)))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("invalid prior lookup journal entry")
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		calls++
		evidenceBytes += info.Size()
		if calls > int64(c.Limits.MaxToolCalls) || evidenceBytes > c.Limits.MaxEvidenceBytes {
			return errors.New("prior lookup journal exceeds the pinned budget")
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "chunsu-mail", Version: "1"}, nil)
	closedWorld, destructive := false, false
	mcp.AddTool(server, &mcp.Tool{Name: ToolName, Description: "Read one source from this immutable mail snapshot. No live APIs, file paths, or write actions are available.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, OpenWorldHint: &closedWorld}}, func(callCtx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) {
		mu.Lock()
		defer mu.Unlock()
		out := Output{}
		if calls >= int64(c.Limits.MaxToolCalls) {
			return nil, Output{Status: "budget_exhausted"}, errors.New("source lookup budget exhausted")
		}
		calls++
		if int64(len(in.SourceID)) > c.Limits.MaxSourceBytes {
			return nil, out, errors.New("invalid source ID length")
		}
		current, e := s.Job(callCtx, jobID)
		if e != nil {
			return nil, out, errors.New("gateway state unavailable")
		}
		if current.Status != store.Running || current.CurrentAttempt != attemptID {
			out = Output{Status: "revoked", Detail: "attempt access revoked"}
		}
		if out.Status == "" && snapshot.Origin != nil {
			connection, e := gmail.LoadConnection(root, snapshot.Origin.ConnectionID, c)
			if e != nil || gmail.PolicyDigest(connection.Policy) != snapshot.Origin.PolicyDigest {
				out = Output{Status: "connection_revoked", Detail: "connection access was revoked or its scope changed"}
			}
		}
		if out.Status == "" {
			if m, ok := sources[in.SourceID]; ok {
				out = Output{Status: "available", Source: &m}
				if m.ContentStatus == "unavailable" {
					out.Status = "unavailable"
				}
			} else {
				out = Output{Status: "outside_scope", Detail: "source is not in this attempt's snapshot"}
			}
		}
		ev := Evidence{Tool: ToolName, SourceID: in.SourceID, At: time.Now().UTC().Format(time.RFC3339Nano), Result: out}
		b, e := json.Marshal(ev)
		if e != nil {
			return nil, out, e
		}
		if int64(len(b)) > c.Limits.MaxArtifactBytes {
			return nil, out, errors.New("lookup evidence exceeds configured limit")
		}
		if int64(len(b)) > c.Limits.MaxEvidenceBytes-evidenceBytes {
			return nil, Output{Status: "budget_exhausted"}, errors.New("lookup evidence byte budget exhausted")
		}
		if e = files.Write(root, filepath.Join(EvidenceDir(jobID, attemptID), files.ID()+".json"), b, false); e != nil {
			return nil, out, fmt.Errorf("cannot preserve lookup evidence: %w", e)
		}
		evidenceBytes += int64(len(b))
		current, e = s.Job(callCtx, jobID)
		if e != nil || current.Status != store.Running || current.CurrentAttempt != attemptID {
			return nil, Output{Status: "revoked", Detail: "attempt access revoked"}, nil
		}
		return nil, out, nil
	})
	return server.Run(ctx, &mcp.StdioTransport{})
}

func verifyJiraManifest(manifest workgroup.Package, input jira.ReportInput) error {
	if manifest.SourceKind != "jira_report_input" || manifest.Synthetic != input.Snapshot.Synthetic {
		return errors.New("gateway Jira source marker differs from the pinned manifest")
	}
	index, err := jira.BuildSourceIndex(input)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if files.Digest(b) != manifest.SourceIndexDigest {
		return errors.New("gateway Jira source index differs from the pinned manifest")
	}
	return nil
}

// serveJira exposes only records already preserved in the pinned report input.
// It intentionally accepts the report-input shape as opaque JSON: normalization
// and policy evolution stay in the Jira adapter while this boundary retains the
// same lifecycle, revocation, byte and evidence controls as mail.
func serveJira(ctx context.Context, root string, s *store.Store, c config.Config, j store.Job, manifest workgroup.Package, input []byte) error {
	reportInput, err := jira.ParseReportInput(input)
	if err != nil {
		return err
	}
	issues, err := jiraIssues(reportInput)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var calls, evidenceBytes int64
	entries, readErr := os.ReadDir(filepath.Join(root, EvidenceDir(j.ID, manifest.AttemptID)))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("invalid prior lookup journal entry")
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		calls++
		evidenceBytes += info.Size()
		if calls > int64(c.Limits.MaxToolCalls) || evidenceBytes > c.Limits.MaxEvidenceBytes {
			return errors.New("prior lookup journal exceeds the pinned budget")
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "chunsu-jira", Version: "1"}, nil)
	closedWorld, destructive := false, false
	mcp.AddTool(server, &mcp.Tool{Name: JiraToolName, Description: "Read one issue from this immutable Jira snapshot. No live Jira APIs, files, or write actions are available.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, OpenWorldHint: &closedWorld}}, func(callCtx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) {
		mu.Lock()
		defer mu.Unlock()
		if calls >= int64(c.Limits.MaxToolCalls) {
			return nil, Output{Status: "budget_exhausted"}, errors.New("source lookup budget exhausted")
		}
		calls++
		out := Output{}
		if int64(len(in.SourceID)) > c.Limits.MaxSourceBytes {
			return nil, out, errors.New("invalid source ID length")
		}
		current, e := s.Job(callCtx, j.ID)
		if e != nil {
			return nil, out, errors.New("gateway state unavailable")
		}
		if current.Status != store.Running || current.CurrentAttempt != manifest.AttemptID {
			out = Output{Status: "revoked", Detail: "attempt access revoked"}
		}
		if out.Status == "" && !reportInput.Snapshot.Synthetic {
			profile, e := jira.LoadProfile(root, reportInput.Policy.ConnectionID, c)
			if e != nil || !profile.Active || profile.SiteHost != reportInput.SiteHost || profile.ReportPolicyDigest() != reportInput.ReportPolicyDigest {
				out = Output{Status: "connection_revoked", Detail: "Jira profile access was revoked or its reviewed report scope changed"}
			}
		}
		if out.Status == "" {
			if issue, ok := issues[in.SourceID]; ok {
				out = Output{Status: "available", Issue: issue}
			} else {
				out = Output{Status: "outside_scope", Detail: "source is not in this attempt's snapshot"}
			}
		}
		ev := Evidence{Tool: JiraToolName, SourceID: in.SourceID, At: time.Now().UTC().Format(time.RFC3339Nano), Result: out}
		b, e := json.Marshal(ev)
		if e != nil {
			return nil, out, e
		}
		if int64(len(b)) > c.Limits.MaxArtifactBytes || int64(len(b)) > c.Limits.MaxEvidenceBytes-evidenceBytes {
			return nil, Output{Status: "budget_exhausted"}, errors.New("lookup evidence byte budget exhausted")
		}
		if e = files.Write(root, filepath.Join(EvidenceDir(j.ID, manifest.AttemptID), files.ID()+".json"), b, false); e != nil {
			return nil, out, fmt.Errorf("cannot preserve lookup evidence: %w", e)
		}
		evidenceBytes += int64(len(b))
		current, e = s.Job(callCtx, j.ID)
		if e != nil || current.Status != store.Running || current.CurrentAttempt != manifest.AttemptID {
			return nil, Output{Status: "revoked", Detail: "attempt access revoked"}, nil
		}
		return nil, out, nil
	})
	return server.Run(ctx, &mcp.StdioTransport{})
}

func jiraIssues(input jira.ReportInput) (map[string]*jiraGatewayIssue, error) {
	out := map[string]*jiraGatewayIssue{}
	for _, issue := range input.Snapshot.Issues {
		if issue.ID == "" || out[issue.ID] != nil {
			return nil, errors.New("invalid Jira source index")
		}
		record := &jiraGatewayIssue{ID: issue.ID, Key: issue.Key, ProjectKey: issue.ProjectKey, Summary: issue.Summary, Status: issue.Status, Assignee: issue.Assignee, CreatedAt: issue.CreatedAt, UpdatedAt: issue.UpdatedAt, ResolvedAt: issue.ResolvedAt, DueDate: issue.DueDate, StartDate: issue.StartDate, Resolution: issue.Resolution}
		if input.Policy.ContentScope == jira.MetadataAndDescription {
			record.Description = &issue.Description
		}
		out[issue.ID] = record
	}
	return out, nil
}

// JiraDisclosure reuses the gateway's exact reviewed field projection for
// isolated result reviewers. It never exposes raw acquisition records.
func JiraDisclosure(input jira.ReportInput) (any, error) { return jiraIssues(input) }

// jiraGatewayIssue is a disclosure projection, not the normalized acquisition
// record. It keeps comments, changelog, relationships, estimates, raw-response
// paths, and descriptions outside the reviewed content scope out of the tool.
type jiraGatewayIssue struct {
	ID          string         `json:"id"`
	Key         string         `json:"key"`
	ProjectKey  string         `json:"project_key"`
	Summary     string         `json:"summary"`
	Status      jira.Status    `json:"status"`
	Assignee    *jira.Identity `json:"assignee,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
	ResolvedAt  string         `json:"resolved_at"`
	DueDate     string         `json:"due_date"`
	StartDate   string         `json:"start_date"`
	Resolution  string         `json:"resolution"`
	Description *jira.Text     `json:"description,omitempty"`
}

func JiraSourceIDs(data []byte) ([]string, error) {
	input, err := jira.ParseReportInput(data)
	if err != nil {
		return nil, err
	}
	issues, err := jiraIssues(input)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(issues))
	for id := range issues {
		out = append(out, id)
	}
	return out, nil
}
