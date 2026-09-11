package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/platform"
	"chunsu/internal/runtimeenv"
)

// ConversationArguments gives the model no native tools. Structured proposals
// are validated and dispatched later by the host, outside this process.
func ConversationArguments(directory, model string) []string {
	return conversationArguments(directory, model, runtimeenv.Boundary{ReadRoots: []string{directory}})
}

func conversationArguments(directory, model string, boundary runtimeenv.Boundary) []string {
	args := []string{"exec", "--json", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--strict-config", "-C", directory, "--output-schema", filepath.Join(directory, "response.schema.json")}
	args = append(args, boundaryArgs(boundary)...)
	args = append(args, "-c", `approval_policy="never"`, "-c", `web_search="disabled"`, "-c", `shell_environment_policy.inherit="none"`, "-c", `mcp_servers={}`, "-c", `project_doc_max_bytes=0`, "-c", `model_reasoning_effort="low"`, "--enable", "skip_host_skill_discovery")
	for _, feature := range disabledFeatures {
		args = append(args, "--disable", feature)
	}
	return append(args, "--model", model, "-")
}

func runCodexStructured(parent context.Context, role, root, directory string, selected config.Executor, limits config.Limits, prompt, schema, skill []byte) (result Result, runErr error) {
	result = Result{ExitCode: -1, Outcome: "not_started", Role: role, Driver: selected.Kind}
	if selected.Kind != "codex" || selected.Path == "" {
		return result, errors.New("AI 실행기가 설정되지 않았습니다. executor.kind/path/model 설정 후 다시 시작하거나 chat --guided를 사용하세요.")
	}
	if selected.Model != TestedModel {
		return result, fmt.Errorf("AI 실행 경계 검증 대상 모델은 %s입니다. 모델 변경은 별도 재검증이 필요합니다.", TestedModel)
	}
	if err := CheckDataRoot(root); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(limits.TimeoutSeconds)*time.Second)
	defer cancel()
	checkCtx, done := context.WithTimeout(ctx, time.Duration(limits.LockWaitSeconds)*time.Second)
	version, err := Version(checkCtx, selected.Path)
	done()
	result.Version = version
	if err != nil {
		return result, err
	}
	if version != TestedVersion {
		return result, fmt.Errorf("AI 실행 경계 검증 대상 실행기는 %s입니다", TestedVersion)
	}
	if err = files.PrivateDir(directory); err != nil {
		return result, err
	}
	for name, data := range map[string][]byte{"response.schema.json": schema, "SKILL.md": skill} {
		if err = files.Write(directory, name, data, false); err != nil {
			return result, err
		}
	}
	environment, err := runtimeenv.Select(selected.Environment)
	if err != nil {
		return result, err
	}
	boundary, err := environment.Boundary(root, directory)
	if err != nil {
		return result, err
	}
	result.Boundary = &boundary
	arguments := conversationArguments(directory, selected.Model, boundary)
	argumentBytes, _ := json.Marshal(arguments)
	result.ArgumentsDigest = files.Digest(argumentBytes)
	defer func() {
		// Audit metadata only: no transcript, provider diagnostic or native tool payload.
		b, e := json.MarshalIndent(struct {
			Result       Result `json:"executor"`
			Model        string `json:"model"`
			ArgsDigest   string `json:"arguments_digest"`
			PromptDigest string `json:"prompt_digest"`
			SchemaDigest string `json:"schema_digest"`
			SkillDigest  string `json:"skill_digest"`
			ReplyDigest  string `json:"reply_digest"`
		}{result, selected.Model, files.Digest(argumentBytes), files.Digest(prompt), files.Digest(schema), files.Digest(skill), files.Digest(result.Final)}, "", "  ")
		if e == nil {
			e = files.Write(directory, "executor.json", b, false)
		}
		if e != nil {
			runErr = errors.Join(runErr, errors.New("대화 실행 증거를 저장하지 못했습니다"))
		}
	}()
	cmd := exec.CommandContext(ctx, selected.Path, arguments...)
	cmd.Dir = directory
	cmd.Env = MinimalEnv()
	cmd.Stdin = strings.NewReader(string(prompt))
	platform.ProcessGroup(cmd)
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return platform.KillGroup(cmd.Process.Pid)
		}
		return nil
	}
	cmd.WaitDelay = time.Duration(limits.LockWaitSeconds) * time.Second
	stderr := &countWriter{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	intent, _ := json.Marshal(ProcessRecord{State: "starting"})
	if err = files.Write(directory, ProcessFile, intent, false); err != nil {
		return result, err
	}
	start := time.Now()
	if err = cmd.Start(); err != nil {
		return result, errors.New("AI 실행기를 시작하지 못했습니다")
	}
	identity, err := platform.Identify(cmd.Process.Pid)
	if err != nil {
		_ = platform.KillGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return result, err
	}
	b, _ := json.Marshal(ProcessRecord{State: "running", Identity: identity})
	if err = files.Write(directory, ProcessFile, b, true); err != nil {
		_ = platform.KillGroup(cmd.Process.Pid)
		_ = cmd.Wait()
		return result, err
	}
	scan := bufio.NewScanner(io.LimitReader(stdout, limits.MaxArtifactBytes+1))
	scan.Buffer(nil, int(limits.MaxArtifactBytes))
	var parseErr error
	var total int64
	completed := false
	failed := false
	for scan.Scan() {
		total += int64(len(scan.Bytes()) + 1)
		if total > limits.MaxArtifactBytes {
			parseErr = errors.New("chat output limit exceeded")
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
		if json.Unmarshal(scan.Bytes(), &event) != nil {
			parseErr = errors.New("invalid chat event stream")
			break
		}
		// Item-level errors can be non-fatal CLI warnings. Only turn/error
		// events and the process exit decide success; warnings grant no tools.
		if event.Item.Type != "" && event.Item.Type != "agent_message" && event.Item.Type != "reasoning" && event.Item.Type != "error" {
			result.ObservedTools = append(result.ObservedTools, event.Item.Type)
			parseErr = errors.New("AI 실행기가 허용되지 않은 도구를 요청했습니다. 작업을 실행하지 않았습니다.")
			break
		}
		switch event.Type {
		case "thread.started":
			result.ThreadID = event.ThreadID
		case "turn.completed":
			completed = true
			result.Usage = event.Usage
		case "error", "turn.failed":
			failed = true
		case "item.completed":
			if event.Item.Type == "agent_message" {
				result.Final = []byte(event.Item.Text)
			}
		}
	}
	if scan.Err() != nil {
		parseErr = errors.New("chat output stream failed")
	}
	if parseErr != nil {
		_ = platform.KillGroup(cmd.Process.Pid)
	}
	err = cmd.Wait()
	result.DurationMS = time.Since(start).Milliseconds()
	result.StderrBytes = stderr.count.Load()
	result.ExitCode = cmd.ProcessState.ExitCode()
	result.Outcome = "failed"
	cleanupDeadline := time.Now().Add(time.Duration(limits.LockWaitSeconds) * time.Second)
	for platform.GroupExists(identity.PID) && time.Now().Before(cleanupDeadline) {
		time.Sleep(time.Duration(limits.PollSeconds) * time.Second)
	}
	if platform.GroupExists(identity.PID) {
		result.Outcome = "orphaned"
		return result, errors.New("AI 실행기 프로세스 정리를 확인해야 합니다")
	}
	b, _ = json.Marshal(ProcessRecord{State: "exited", Identity: identity})
	if e := files.Write(directory, ProcessFile, b, true); e != nil {
		return result, e
	}
	if ctx.Err() != nil {
		result.Outcome = "interrupted"
		return result, ctx.Err()
	}
	if parseErr != nil {
		return result, parseErr
	}
	if err != nil || failed || !completed || len(result.Final) == 0 {
		return result, errors.New("AI 실행기가 답변을 완료하지 못했습니다. Codex 로그인·사용량·연결 상태를 확인하세요. 보존된 실행 기록을 확인하세요.")
	}
	result.Outcome = "generated"
	return result, nil
}
