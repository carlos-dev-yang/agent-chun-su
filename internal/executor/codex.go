package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/workgroup"
)

const TestedVersion = "codex-cli 0.153.4"
const Profile = "chunsu_mail"
const ProcessFile = "process.json"
const MaxVersionBytes = 4096

type ProcessRecord struct {
	State    string                   `json:"state"`
	Identity platform.ProcessIdentity `json:"identity"`
}

var disabledFeatures = []string{
	"apps", "plugins", "plugin_sharing", "recommended_plugins", "hooks",
	"browser_use", "browser_use_external", "browser_use_full_cdp_access", "computer_use",
	"in_app_browser", "in_app_local_automation", "in_app_chat", "in_app_dictation", "in_app_updates",
	"image_generation", "multi_agent", "multi_agent_v2", "remote_plugin", "goals",
	"memories", "chronicle", "external_agent_memory_import", "enable_mcp_apps",
	"shell_snapshot", "shell_tool", "skill_mcp_dependency_install", "skill_search",
	"sleep_tool", "tool_suggest", "unified_exec", "view_image", "workspace_dependencies",
	"code_mode_host", "code_mode", "code_mode_only", "deferred_executor", "standalone_web_search",
	"auth_elicitation", "tool_call_mcp_elicitation", "request_permissions_tool", "unbounded_connection_retries",
}

type Result struct {
	Final       []byte          `json:"-"`
	ExitCode    int             `json:"exit_code"`
	DurationMS  int64           `json:"duration_ms"`
	ThreadID    string          `json:"thread_id,omitempty"`
	Usage       json.RawMessage `json:"usage,omitempty"`
	StderrBytes int64           `json:"stderr_bytes"`
	Outcome     string          `json:"outcome"`
	Version     string          `json:"version"`
}

func MinimalEnv() []string {
	keys := []string{"HOME", "PATH", "TMPDIR", "LANG", "LC_ALL", "CODEX_HOME", "SSL_CERT_FILE", "SSL_CERT_DIR"}
	out := []string{}
	for _, key := range keys {
		if v, ok := os.LookupEnv(key); ok {
			out = append(out, key+"="+v)
		}
	}
	return out
}

func ProfileArgs(directory string) []string {
	fs := fmt.Sprintf("{ %s = %s, %s = %s }", strconv.Quote(":minimal"), strconv.Quote("read"), strconv.Quote(directory), strconv.Quote("read"))
	return []string{"-c", "default_permissions=" + strconv.Quote(Profile), "-c", "permissions." + Profile + ".filesystem=" + fs, "-c", "permissions." + Profile + ".network.enabled=false"}
}

func Version(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = MinimalEnv()
	buffer := &limitedBuffer{limit: MaxVersionBytes}
	cmd.Stdout = buffer
	err := cmd.Run()
	if err != nil {
		return "", errors.New("cannot inspect executor version")
	}
	if buffer.overflow {
		return "", errors.New("invalid executor version output")
	}
	return strings.TrimSpace(buffer.String()), nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if n > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func Arguments(binary, root string, p workgroup.Package) []string {
	args := []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--strict-config", "-C", p.Directory, "--output-schema", filepath.Join(p.Directory, "report.schema.json")}
	args = append(args, ProfileArgs(p.Directory)...)
	args = append(args, "-c", `approval_policy="never"`, "-c", `web_search="disabled"`, "-c", `shell_environment_policy.inherit="none"`, "--enable", "skip_host_skill_discovery")
	for _, f := range disabledFeatures {
		args = append(args, "--disable", f)
	}
	serverArgs := []string{"--home", root, "tools", "serve", p.JobID, p.AttemptID}
	quoted := []string{}
	for _, a := range serverArgs {
		quoted = append(quoted, strconv.Quote(a))
	}
	server := fmt.Sprintf("{ command = %s, args = [%s], enabled = true, required = true, enabled_tools = [%s] }", strconv.Quote(binary), strings.Join(quoted, ","), strconv.Quote("mail_source_get"))
	args = append(args, "-c", "mcp_servers={ chunsu_mail = "+server+" }")
	if p.Executor.Model != "" {
		args = append(args, "--model", p.Executor.Model)
	}
	return append(args, "-")
}

type countWriter struct{ count atomic.Int64 }

func (w *countWriter) Write(p []byte) (int, error) { w.count.Add(int64(len(p))); return len(p), nil }

func Run(ctx context.Context, root string, p workgroup.Package) (Result, error) {
	result := Result{ExitCode: -1, Outcome: "not_started"}
	if p.Executor.Kind != "codex" || p.Executor.Path == "" {
		return result, errors.New("configure the selected Codex executor before running")
	}
	if p.Snapshot.Origin != nil && !p.Snapshot.Synthetic && !p.Executor.LiveMailApproved {
		return result, errors.New("live-mail disclosure is disabled; validate the actual executor boundary with synthetic sources, then explicitly approve this executor configuration")
	}
	versionCtx, versionCancel := context.WithTimeout(ctx, time.Duration(p.Limits.LockWaitSeconds)*time.Second)
	version, err := Version(versionCtx, p.Executor.Path)
	versionCancel()
	result.Version = version
	if err != nil {
		return result, err
	}
	if version != TestedVersion {
		return result, fmt.Errorf("executor version %q needs boundary revalidation; supported version is %s", version, TestedVersion)
	}
	loginCtx, loginCancel := context.WithTimeout(ctx, time.Duration(p.Limits.LockWaitSeconds)*time.Second)
	defer loginCancel()
	login := exec.CommandContext(loginCtx, p.Executor.Path, "login", "status")
	login.Env = MinimalEnv()
	if err = login.Run(); err != nil {
		result.Outcome = "waiting_auth"
		return result, errors.New("Codex authentication is unavailable; sign in with the executor's own login command")
	}
	executable, err := os.Executable()
	if err != nil {
		return result, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return result, err
	}
	instructions, err := files.Read(p.Directory, "instructions.md", p.Limits.MaxArtifactBytes)
	if err != nil {
		return result, err
	}
	for name, digest := range p.Files {
		b, e := files.Read(p.Directory, name, p.Limits.MaxArtifactBytes)
		if e != nil || files.Digest(b) != digest {
			return result, errors.New("pinned package integrity mismatch")
		}
	}
	cmd := exec.CommandContext(ctx, p.Executor.Path, Arguments(executable, root, p)...)
	cmd.Dir = p.Directory
	cmd.Env = MinimalEnv()
	cmd.Stdin = strings.NewReader(string(instructions))
	platform.ProcessGroup(cmd)
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return platform.KillGroup(cmd.Process.Pid)
		}
		return nil
	}
	stderr := &countWriter{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	start := time.Now()
	processPath := filepath.Join("runs", p.JobID, "attempts", p.AttemptID, ProcessFile)
	intent, _ := json.Marshal(ProcessRecord{State: "starting"})
	if err = files.Write(root, processPath, intent, false); err != nil {
		return result, err
	}
	if err = cmd.Start(); err != nil {
		ended, _ := json.Marshal(ProcessRecord{State: "not_started"})
		_ = files.Write(root, processPath, ended, true)
		return result, errors.New("executor could not start")
	}
	identity, err := platform.Identify(cmd.Process.Pid)
	if err != nil {
		_ = platform.KillGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return result, errors.New("could not record executor process identity")
	}
	processData, _ := json.Marshal(ProcessRecord{State: "running", Identity: identity})
	if err = files.Write(root, processPath, processData, true); err != nil {
		_ = platform.KillGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return result, err
	}
	scan := bufio.NewScanner(io.LimitReader(stdout, p.Limits.MaxArtifactBytes+1))
	scan.Buffer(make([]byte, bufio.MaxScanTokenSize), int(p.Limits.MaxArtifactBytes))
	var total int64
	var parseErr error
	for scan.Scan() {
		line := scan.Bytes()
		total += int64(len(line) + 1)
		if total > p.Limits.MaxArtifactBytes {
			parseErr = errors.New("executor output budget exceeded")
			break
		}
		var event struct {
			Type     string          `json:"type"`
			ThreadID string          `json:"thread_id"`
			Usage    json.RawMessage `json:"usage"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if e := json.Unmarshal(line, &event); e != nil {
			parseErr = errors.New("invalid executor event stream")
			break
		}
		switch event.Type {
		case "thread.started":
			result.ThreadID = event.ThreadID
		case "turn.completed":
			result.Usage = event.Usage
		case "item.completed":
			if event.Item.Type == "agent_message" {
				result.Final = []byte(event.Item.Text)
			}
		}
		// Reasoning and tool item payloads are deliberately not persisted.
	}
	if e := scan.Err(); e != nil {
		parseErr = errors.New("executor event stream exceeded limits or failed")
	}
	if parseErr != nil {
		_ = platform.KillGroup(cmd.Process.Pid)
	}
	err = cmd.Wait()
	result.DurationMS = time.Since(start).Milliseconds()
	result.StderrBytes = stderr.count.Load()
	result.ExitCode = cmd.ProcessState.ExitCode()
	result.Outcome = "exited"
	cleanupDeadline := time.Now().Add(time.Duration(p.Limits.LockWaitSeconds) * time.Second)
	for platform.GroupExists(identity.PID) && time.Now().Before(cleanupDeadline) {
		time.Sleep(time.Duration(p.Limits.PollSeconds) * time.Second)
	}
	if platform.GroupExists(identity.PID) {
		result.Outcome = "orphaned"
		return result, errors.New("executor descendants remain; inspect process identity before recovery")
	}
	processData, _ = json.Marshal(ProcessRecord{State: "exited", Identity: identity})
	if writeErr := files.Write(root, processPath, processData, true); writeErr != nil {
		return result, writeErr
	}
	if ctx.Err() != nil {
		result.Outcome = "interrupted"
		return result, ctx.Err()
	}
	if parseErr != nil {
		result.Outcome = "invalid_stream"
		return result, parseErr
	}
	if err != nil {
		result.Outcome = "failed"
		return result, fmt.Errorf("executor exited with code %d; raw diagnostic content was not retained", result.ExitCode)
	}
	if len(result.Final) == 0 {
		return result, errors.New("executor returned no final structured result")
	}
	result.Outcome = "generated"
	return result, nil
}
