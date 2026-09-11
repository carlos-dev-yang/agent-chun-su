package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type SnapshotRecord struct {
	ID   string         `json:"id"`
	Data map[string]any `json:"data"`
}

func serveSnapshot(ctx context.Context, root string, s *store.Store, c config.Config, job store.Job, manifest workgroup.Package, input []byte, definition workgroup.Definition) error {
	sources, err := definition.SnapshotSources(input, c.Limits)
	if err != nil {
		return err
	}
	plan, err := definition.PrepareInput(&manifest, input)
	if err != nil {
		return err
	}
	if manifest.SourceKind != definition.SourceKind || files.Digest(plan.Index) != manifest.SourceIndexDigest {
		return errors.New("snapshot gateway index differs from the pinned manifest")
	}
	var mu sync.Mutex
	var calls int
	var total int64
	directory := EvidenceDir(job.ID, manifest.AttemptID)
	entries, err := os.ReadDir(filepath.Join(root, directory))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("invalid lookup journal entry")
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		calls++
		total += info.Size()
	}
	if calls > c.Limits.MaxToolCalls || total > c.Limits.MaxEvidenceBytes {
		return errors.New("lookup journal exceeds the pinned budget")
	}
	authorized := func(callCtx context.Context) bool {
		current, e := s.Job(callCtx, job.ID)
		if e != nil || current.Status != store.Running || current.CurrentAttempt != manifest.AttemptID {
			return false
		}
		if definition.AuthorizeInput != nil {
			currentConfig, e := config.Load(root)
			if e != nil {
				return false
			}
			if e = definition.AuthorizeInput(input, c.Limits, currentConfig.Executor); e != nil {
				return false
			}
		}
		return true
	}
	server := mcp.NewServer(&mcp.Implementation{Name: definition.ID, Version: "1"}, nil)
	closedWorld, destructive := false, false
	mcp.AddTool(server, &mcp.Tool{Name: definition.Tool, Description: "Read one exact source from the selected immutable snapshot. No repository access, paths, commands, network or mutations are accepted.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, OpenWorldHint: &closedWorld}}, func(callCtx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) {
		mu.Lock()
		defer mu.Unlock()
		if calls >= c.Limits.MaxToolCalls || int64(len(in.SourceID)) > c.Limits.MaxSourceBytes {
			return nil, Output{Status: "budget_exhausted"}, errors.New("source lookup budget exhausted")
		}
		calls++
		out := Output{Status: "revoked"}
		if authorized(callCtx) {
			out.Status = "outside_scope"
			if data, ok := sources[in.SourceID]; ok {
				var record map[string]any
				if e := json.Unmarshal(data, &record); e != nil {
					return nil, Output{}, e
				}
				out = Output{Status: "available", Record: &SnapshotRecord{ID: in.SourceID, Data: record}}
			}
		}
		b, e := json.Marshal(Evidence{Tool: definition.Tool, SourceID: in.SourceID, At: time.Now().UTC().Format(time.RFC3339Nano), Result: out})
		if e != nil {
			return nil, out, e
		}
		if int64(len(b)) > c.Limits.MaxArtifactBytes || int64(len(b)) > c.Limits.MaxEvidenceBytes-total {
			return nil, Output{Status: "budget_exhausted"}, errors.New("lookup evidence budget exhausted")
		}
		if e = files.Write(root, filepath.Join(directory, files.ID()+".json"), b, false); e != nil {
			return nil, Output{}, e
		}
		total += int64(len(b))
		if !authorized(callCtx) {
			return nil, Output{Status: "revoked"}, nil
		}
		return nil, out, nil
	})
	return server.Run(ctx, &mcp.StdioTransport{})
}
