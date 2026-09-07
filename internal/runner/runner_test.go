package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/mail"
	"chunsu/internal/store"
	"chunsu/internal/workgroup"
)

func TestRunExecutionGates(t *testing.T) {
	tests := []struct {
		name           string
		item           map[string]any
		withTarget     bool
		exitFailure    bool
		wantStatus     string
		wantDiagnostic string
	}{
		{
			name:       "allows the scoped mail tool",
			item:       map[string]any{"type": "mcp_tool_call", "server": "chunsu_mail", "tool": "mail_source_get"},
			wantStatus: store.Completed,
		},
		{
			name:       "rejects unexpected MCP tools without retry",
			item:       map[string]any{"type": "mcp_tool_call", "server": "unexpected", "tool": "tool"},
			wantStatus: store.Failed, wantDiagnostic: executor.CapabilityViolation,
		},
		{
			name:        "rejects command execution without retry",
			item:        map[string]any{"type": "command_execution"},
			exitFailure: true,
			wantStatus:  store.Failed, wantDiagnostic: executor.CapabilityViolation,
		},
		{
			name:       "rejects file changes without retry",
			item:       map[string]any{"type": "file_change"},
			wantStatus: store.Failed, wantDiagnostic: executor.CapabilityViolation,
		},
		{
			name:       "rejects web search without retry",
			item:       map[string]any{"type": "web_search"},
			wantStatus: store.Failed, wantDiagnostic: executor.CapabilityViolation,
		},
		{
			name:       "rejects a successful result missing target lookup evidence",
			item:       map[string]any{"type": "mcp_tool_call", "server": "chunsu_mail", "tool": "mail_source_get"},
			withTarget: true, wantStatus: store.Failed, wantDiagnostic: requiredSourceLookupsMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := runnerTestRoot(t)
			c := config.Defaults()
			c.Executor = config.Executor{Kind: "codex", Path: writeFakeExecutor(t, root, tt.item, tt.withTarget, tt.exitFailure), Model: executor.TestedModel}
			if _, err := config.Setup(root); err != nil {
				t.Fatal(err)
			}
			if err := workgroup.Install(root, c.Limits.MaxArtifactBytes); err != nil {
				t.Fatal(err)
			}
			s, err := store.Open(context.Background(), root, c, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			input, err := json.Marshal(testSnapshot(tt.withTarget))
			if err != nil {
				t.Fatal(err)
			}
			job, err := s.Submit(context.Background(), mail.Workgroup, input, map[string]any{}, c.Limits.MaxArtifactBytes)
			if err != nil {
				t.Fatal(err)
			}
			out, runErr := (&Runner{Store: s, Config: c}).Run(context.Background(), job.ID, "")
			if out.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q, diagnostic=%q error=%v", out.Status, tt.wantStatus, out.Diagnostic, runErr)
			}
			if tt.wantDiagnostic != "" {
				if out.Diagnostic != tt.wantDiagnostic || runErr == nil || !strings.Contains(runErr.Error(), tt.wantDiagnostic) {
					t.Fatalf("diagnostic was not reported: outcome=%+v error=%v", out, runErr)
				}
			} else if runErr != nil {
				t.Fatal(runErr)
			}
			current, err := s.Job(context.Background(), job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != tt.wantStatus || current.Diagnostic != tt.wantDiagnostic {
				t.Fatalf("stored job = %+v, want status=%q diagnostic=%q", current, tt.wantStatus, tt.wantDiagnostic)
			}
			artifacts, err := s.Artifacts(context.Background(), job.ID)
			if err != nil {
				t.Fatal(err)
			}
			kinds := map[string]bool{}
			for _, artifact := range artifacts {
				kinds[artifact.Kind] = true
			}
			if !kinds["executor_result"] || !kinds["raw_result"] {
				t.Fatalf("result or raw output was not preserved: %v", kinds)
			}
			shouldValidate := tt.wantDiagnostic != executor.CapabilityViolation
			if kinds["validation"] != shouldValidate {
				t.Fatalf("validation preserved=%t, want %t: %v", kinds["validation"], shouldValidate, kinds)
			}
			if tt.exitFailure {
				var generated executor.Result
				for _, artifact := range artifacts {
					if artifact.Kind != "executor_result" {
						continue
					}
					b, err := s.ReadArtifact(artifact, c.Limits.MaxArtifactBytes)
					if err != nil {
						t.Fatal(err)
					}
					if err = mail.Decode(b, &generated); err != nil {
						t.Fatal(err)
					}
				}
				if !mail.Contains(generated.ObservedTools, "command_execution") {
					t.Fatalf("capability evidence = %v, want command_execution", generated.ObservedTools)
				}
				events, err := s.Events(context.Background(), job.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range events {
					if event.Kind == "result.generated" {
						t.Fatal("failed executor output recorded result.generated")
					}
				}
			}
			if tt.wantDiagnostic != "" && kinds["report_markdown"] {
				t.Fatal("execution gate published a report")
			}
			attempts, err := s.Attempts(context.Background(), job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(attempts) != 1 || attempts[0].Status != tt.wantStatus {
				t.Fatalf("attempts = %+v, want one %s attempt", attempts, tt.wantStatus)
			}
		})
	}
}

func testSnapshot(withTarget bool) mail.Snapshot {
	snapshot := mail.Snapshot{
		Version: mail.Version, Synthetic: true, AsOf: "2026-09-07T00:00:00Z", Timezone: "UTC",
		Collection: mail.Collection{Status: "complete", Errors: []string{}}, Messages: []mail.Message{}, PriorInterpretations: []mail.Interpretation{},
	}
	if withTarget {
		snapshot.Messages = append(snapshot.Messages, mail.Message{
			ID: "mail-1", ThreadID: "thread-1", ReceivedAt: snapshot.AsOf, Scope: mail.Target, Channel: "email",
			From: "sender@example.test", Subject: "Subject", Body: "Body", ContentStatus: "complete", Attachments: []mail.Attachment{},
		})
	}
	return snapshot
}

func runnerTestRoot(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		return t.TempDir()
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(wd, ".runner-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func writeFakeExecutor(t *testing.T, root string, item map[string]any, withTarget, exitFailure bool) string {
	t.Helper()
	dispositions := []any{}
	if withTarget {
		dispositions = append(dispositions, map[string]any{"source_id": "mail-1", "disposition": "excluded", "item_ids": []any{}, "reason": "No action needed"})
	}
	report := map[string]any{
		"version": 1, "as_of": "2026-09-07T00:00:00Z", "timezone": "UTC", "summary": "No messages in scope",
		"items": []any{}, "dispositions": dispositions, "limitations": []any{}, "questions": []any{},
	}
	final, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(map[string]any{"type": "item.completed", "item": item})
	if err != nil {
		t.Fatal(err)
	}
	message, err := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": string(final)}})
	if err != nil {
		t.Fatal(err)
	}
	exit := ""
	if exitFailure {
		exit = "\nexit 1"
	}
	script := "#!/bin/sh\ncase \"$1\" in\n--version) printf '%s\n' '" + executor.TestedVersion + "' ;;\nlogin) exit 0 ;;\nexec) printf '%s\n' '" + string(event) + "' '" + string(message) + "'" + exit + " ;;\nesac\n"
	path := filepath.Join(root, "fake-codex")
	if err = os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
