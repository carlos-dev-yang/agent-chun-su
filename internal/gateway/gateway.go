package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolName = "mail_source_get"

type Input struct {
	SourceID string `json:"source_id" jsonschema:"Exact source ID from this attempt's source index"`
}
type Output struct {
	Status string        `json:"status"`
	Source *mail.Message `json:"source,omitempty"`
	Detail string        `json:"detail,omitempty"`
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
			b, e := s.ReadArtifact(art, c.Limits.MaxArtifactBytes)
			if e != nil {
				return e
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
	sources := map[string]mail.Message{}
	for _, m := range snapshot.Messages {
		sources[m.ID] = m
	}
	var calls atomic.Int64
	var evidenceBytes atomic.Int64
	server := mcp.NewServer(&mcp.Implementation{Name: "chunsu-mail", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: ToolName, Description: "Read one source from this immutable mail snapshot. No live APIs, file paths, or write actions are available."}, func(callCtx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) {
		out := Output{}
		current, e := s.Job(callCtx, jobID)
		if e != nil {
			return nil, out, errors.New("gateway state unavailable")
		}
		if current.Status != store.Running || current.CurrentAttempt != attemptID {
			return nil, Output{Status: "revoked"}, errors.New("attempt access revoked")
		}
		if snapshot.Origin != nil {
			connection, e := gmail.LoadConnection(root, snapshot.Origin.ConnectionID, c)
			if e != nil || gmail.PolicyDigest(connection.Policy) != snapshot.Origin.PolicyDigest {
				return nil, Output{Status: "connection_revoked"}, errors.New("connection access was revoked or its scope changed")
			}
		}
		if calls.Add(1) > int64(c.Limits.MaxToolCalls) {
			return nil, Output{Status: "budget_exhausted"}, errors.New("source lookup budget exhausted")
		}
		if m, ok := sources[in.SourceID]; ok {
			out = Output{Status: "available", Source: &m}
			if m.ContentStatus == "unavailable" {
				out.Status = "unavailable"
			}
		} else {
			out = Output{Status: "outside_scope", Detail: "source is not in this attempt's snapshot"}
		}
		if int64(len(in.SourceID)) > c.Limits.MaxSourceBytes {
			return nil, out, errors.New("invalid source ID length")
		}
		ev := Evidence{Tool: ToolName, SourceID: in.SourceID, At: time.Now().UTC().Format(time.RFC3339Nano), Result: out}
		b, e := json.Marshal(ev)
		if e != nil {
			return nil, out, e
		}
		if int64(len(b)) > c.Limits.MaxArtifactBytes {
			return nil, out, errors.New("lookup evidence exceeds configured limit")
		}
		if evidenceBytes.Add(int64(len(b))) > c.Limits.MaxEvidenceBytes {
			return nil, Output{Status: "budget_exhausted"}, errors.New("lookup evidence byte budget exhausted")
		}
		if e = files.Write(root, filepath.Join(EvidenceDir(jobID, attemptID), files.ID()+".json"), b, false); e != nil {
			return nil, out, fmt.Errorf("cannot preserve lookup evidence: %w", e)
		}
		current, e = s.Job(callCtx, jobID)
		if e != nil || current.Status != store.Running || current.CurrentAttempt != attemptID {
			return nil, Output{Status: "revoked"}, errors.New("attempt access revoked")
		}
		return nil, out, nil
	})
	return server.Run(ctx, &mcp.StdioTransport{})
}
