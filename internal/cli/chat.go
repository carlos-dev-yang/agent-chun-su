package cli

import (
	"bufio"
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
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/onboarding"
	"chunsu/internal/runner"
	"github.com/spf13/cobra"
)

const setupStartupPoll = 50 * time.Millisecond

var errSetupBack = errors.New("return to setup menu")

func (o *options) setupStop() *cobra.Command {
	return &cobra.Command{Use: "stop", Short: "Stop a standalone setup host; never stop a report worker", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		result, err := setupCall(cmd.Context(), root, c, "stop", onboarding.Request{})
		if err != nil {
			return err
		}
		return output(cmd, result)
	}}
}

func (o *options) setupTask(cancelTask bool) *cobra.Command {
	operation := "status"
	if cancelTask {
		operation = "cancel"
	}
	return &cobra.Command{Use: operation + " TASK_ID", Short: "Inspect or cancel a setup task without repeating authorization", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if err != nil {
			return err
		}
		input := onboarding.Request{ID: args[0]}
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		req := control.Request{Operation: onboarding.Prefix + operation, Input: b}
		data, handled, err := control.Call(cmd.Context(), root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, req)
		if err != nil {
			return err
		}
		var result onboarding.Result
		if handled {
			if err = json.Unmarshal(data, &result); err != nil {
				return err
			}
		} else {
			host := &onboarding.Host{Root: root, Config: c}
			value, e := host.Handle(cmd.Context(), req)
			if e != nil {
				return e
			}
			result = value.(onboarding.Result)
		}
		// Status inspection never prints a live OAuth URL into diagnostic output.
		result.AuthURL = ""
		return output(cmd, result)
	}}
}

// setupServe is a host-only management process. It runs no AI jobs or schedules.
func (o *options) setupServe() *cobra.Command {
	return &cobra.Command{Use: "serve", Short: "Run the local setup host until Ctrl-C; no AI executor required", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, c, closeStore, err := o.open(cmd.Context(), true)
		if err != nil {
			return err
		}
		defer closeStore()
		r := &runner.Runner{Store: s, Config: c}
		defer r.CloseSetup()
		stopped := make(chan struct{})
		var once sync.Once
		handle := func(ctx context.Context, req control.Request) (any, error) {
			if req.Operation == onboarding.Prefix+"stop" {
				once.Do(func() { close(stopped) })
				return onboarding.Result{Status: "stopping", Message: "설정 전용 프로세스를 종료합니다."}, nil
			}
			return r.Handle(ctx, req)
		}
		server, err := control.Listen(cmd.Context(), s.Root, c.Limits.MaxArtifactBytes, time.Duration(c.Limits.LockWaitSeconds)*time.Second, handle)
		if err != nil {
			return err
		}
		defer server.Close()
		select {
		case <-cmd.Context().Done():
		case <-stopped:
		}
		return nil
	}}
}

func (o *options) chat() *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{Use: "chat [REQUEST]", Short: "Guided setup conversation with a separate host process", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := o.path()
		if err != nil {
			return err
		}
		c, err := config.Load(root)
		if errors.Is(err, os.ErrNotExist) {
			setup := o.setup()
			setup.SetContext(cmd.Context())
			setup.SetOut(cmd.OutOrStdout())
			if err = setup.RunE(setup, nil); err != nil {
				return err
			}
			c, err = config.Load(root)
		}
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		stopHost, err := startSetupHost(ctx, cmd, root, c)
		if err != nil {
			return err
		}
		defer stopHost()
		d := &setupDialogue{ctx: ctx, root: root, config: c, out: cmd.OutOrStdout(), noBrowser: noBrowser}
		d.lines = readSetupLines(ctx, cmd.InOrStdin(), c.Limits.MaxSourceBytes)
		fmt.Fprintln(d.out, "춘수 설정 대화입니다. 서비스 이름이나 ‘Gmail 연결해줘’처럼 입력하세요.")
		fmt.Fprintln(d.out, "정해진 설치 절차를 진행합니다. 토큰이나 비밀값은 입력하지 마세요. 뒤로/취소: 서비스 선택, 종료: 대화 종료.")
		initial := ""
		if len(args) != 0 {
			initial = args[0]
		}
		err = d.run(initial)
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Show Google sign-in URL locally instead of opening the browser")
	return cmd
}

func startSetupHost(ctx context.Context, cmd *cobra.Command, root string, c config.Config) (func(), error) {
	check := func() (bool, error) {
		_, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
		return handled, err
	}
	if handled, err := check(); handled || err != nil {
		return func() {}, err
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	child := exec.CommandContext(ctx, binary, "--home", root, "setup", "serve")
	child.Stderr = cmd.ErrOrStderr()
	child.Cancel = func() error { return child.Process.Signal(os.Interrupt) }
	child.WaitDelay = time.Duration(gmail.DefaultHTTPTimeoutSeconds+c.Limits.LockWaitSeconds) * time.Second
	if err = child.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	stop := func() { _ = child.Process.Signal(os.Interrupt); <-done }
	timer := time.NewTimer(time.Duration(c.Limits.LockWaitSeconds) * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(setupStartupPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return nil, ctx.Err()
		case <-timer.C:
			stop()
			return nil, errors.New("설정 프로세스에 연결하지 못했습니다. 같은 데이터 경로의 실행 상태를 확인하세요.")
		case err := <-done:
			if err == nil {
				err = errors.New("설정 프로세스가 준비 전에 종료되었습니다")
			}
			return nil, err
		case <-ticker.C:
			if handled, err := check(); err != nil {
				stop()
				return nil, err
			} else if handled {
				return stop, nil
			}
		}
	}
}

type setupLine struct {
	text string
	err  error
}

func readSetupLines(ctx context.Context, in io.Reader, limit int64) <-chan setupLine {
	ch := make(chan setupLine)
	go func() {
		defer close(ch)
		scan := bufio.NewScanner(in)
		scan.Buffer(nil, int(limit))
		for scan.Scan() {
			select {
			case ch <- setupLine{text: strings.TrimSpace(scan.Text())}:
			case <-ctx.Done():
				return
			}
		}
		if err := scan.Err(); err != nil {
			select {
			case ch <- setupLine{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return ch
}

type setupDialogue struct {
	ctx       context.Context
	root      string
	config    config.Config
	out       io.Writer
	lines     <-chan setupLine
	noBrowser bool
}

func (d *setupDialogue) ask(prompt string) (string, error) {
	fmt.Fprintln(d.out, prompt)
	fmt.Fprint(d.out, "> ")
	select {
	case <-d.ctx.Done():
		return "", d.ctx.Err()
	case line, ok := <-d.lines:
		if !ok {
			return "", io.EOF
		}
		if line.err != nil {
			return "", line.err
		}
		switch strings.ToLower(line.text) {
		case "종료", "그만", "quit", "exit":
			return "", io.EOF
		case "취소", "뒤로", "cancel", "back":
			return "", errSetupBack
		}
		return line.text, nil
	}
}

func (d *setupDialogue) call(operation string, input onboarding.Request) (onboarding.Result, error) {
	return setupCall(d.ctx, d.root, d.config, operation, input)
}

func setupCall(ctx context.Context, root string, c config.Config, operation string, input onboarding.Request) (onboarding.Result, error) {
	var result onboarding.Result
	b, err := json.Marshal(input)
	if err != nil {
		return result, err
	}
	data, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: onboarding.Prefix + operation, Input: b})
	if err != nil {
		return result, err
	}
	if !handled {
		return result, errors.New("설정 프로세스가 중단되었습니다. 연결 목록을 확인한 뒤 대화를 다시 시작하세요.")
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

func (d *setupDialogue) run(initial string) error {
	services, err := onboarding.Services()
	if err != nil {
		return err
	}
	for {
		request := initial
		initial = ""
		if request == "" {
			for i, service := range services {
				fmt.Fprintf(d.out, "%d. %s\n", i+1, service.Name)
			}
			request, err = d.ask("어떤 서비스를 연결할까요?")
			if errors.Is(err, errSetupBack) {
				continue
			}
			if err != nil {
				return err
			}
		}
		selected := -1
		if n, e := strconv.Atoi(request); e == nil && n > 0 && n <= len(services) {
			selected = n - 1
		} else {
			for i, service := range services {
				matched := strings.Contains(strings.ToLower(request), service.ID)
				for _, alias := range service.Aliases {
					matched = matched || strings.Contains(strings.ToLower(request), alias)
				}
				if matched {
					if selected != -1 {
						selected = -1
						break
					}
					selected = i
				}
			}
		}
		if selected < 0 {
			fmt.Fprintln(d.out, "서비스 하나를 이름이나 번호로 선택해 주세요. 아직 일반 업무 대화는 지원하지 않습니다.")
			continue
		}
		service := services[selected]
		prepared, e := d.call("prepare", onboarding.Request{Service: service.ID})
		if e != nil {
			return e
		}
		fmt.Fprintln(d.out, prepared.Message)
		fmt.Fprintln(d.out, "매뉴얼:", prepared.ManualPath)
		if service.ID == "gmail" {
			err = d.gmail()
		} else {
			err = d.guidance(prepared)
		}
		if errors.Is(err, errSetupBack) {
			fmt.Fprintln(d.out, "서비스 선택으로 돌아갑니다.")
			continue
		}
		if err != nil {
			return err
		}
	}
}

func (d *setupDialogue) guidance(result onboarding.Result) error {
	fmt.Fprintln(d.out, "이 서비스는 설정 자료 설치까지 지원합니다. 외부 프로그램 설치와 계정 연결 완료로 표시하지 않습니다.")
	answer, err := d.ask("설치 안내 내용을 볼까요? ‘안내’ 또는 ‘뒤로’를 입력하세요.")
	if err != nil {
		return err
	}
	if answer == "안내" || strings.EqualFold(answer, "help") {
		b, e := files.Read(filepath.Dir(result.ManualPath), filepath.Base(result.ManualPath), d.config.Limits.MaxArtifactBytes)
		if e != nil {
			return e
		}
		fmt.Fprintln(d.out, string(b))
	}
	return nil
}

func (d *setupDialogue) gmail() error {
	entries, err := os.ReadDir(filepath.Join(d.root, "state", "connections"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var connections []gmail.Connection
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		connection, e := gmail.ReadConnection(d.root, strings.TrimSuffix(entry.Name(), ".json"), d.config)
		if e != nil {
			return errors.New("기존 Gmail 연결을 읽을 수 없습니다. 설정을 확인하세요.")
		}
		if connection.Enabled {
			connections = append(connections, connection)
		}
	}
	if len(connections) > 0 {
		for i, c := range connections {
			fmt.Fprintf(d.out, "%d. %s — %s (저장된 연결; 이번 세션 계정 확인 전)\n", i+1, c.Account, c.Policy.Query)
		}
		answer, e := d.ask("기존 연결을 확인해 재사용하려면 번호, 다른 계정은 ‘새 연결’을 입력하세요.")
		if e != nil {
			return e
		}
		if n, e := strconv.Atoi(answer); e == nil && n > 0 && n <= len(connections) {
			return d.startAndWait("gmail.check", onboarding.Request{ID: files.ID(), ConnectionID: connections[n-1].ID})
		}
		if answer != "새 연결" {
			fmt.Fprintln(d.out, "선택을 확인할 수 없어 로그인은 시작하지 않았습니다.")
			return nil
		}
	}
	var account, client, query string
	for {
		account, err = d.ask("연결할 Gmail 주소를 입력해 주세요.")
		if err != nil {
			return err
		}
		if e := gmail.CheckAccount(account, account); e != nil {
			fmt.Fprintln(d.out, "이메일 주소만 입력해 주세요.")
			continue
		}
		break
	}
	for {
		client, err = d.ask("Google Desktop OAuth client JSON 파일 경로를 입력해 주세요. 파일이 없으면 ‘도움말’을 입력하세요.")
		if err != nil {
			return err
		}
		if client == "도움말" {
			fmt.Fprintln(d.out, "Google Cloud 프로젝트에서 Gmail API를 켜고 동의 화면을 설정한 뒤 Desktop app OAuth client를 만들어 JSON을 내려받으세요.")
			fmt.Fprintln(d.out, "설치된 매뉴얼 옆 google-auth.md에 단계별 안내가 있습니다. JSON 내용은 대화에 붙여넣지 마세요.")
			continue
		}
		client = strings.Trim(client, "\"'")
		if strings.HasPrefix(client, "~/") {
			home, e := os.UserHomeDir()
			if e != nil {
				return e
			}
			client = filepath.Join(home, client[2:])
		}
		client, err = filepath.Abs(client)
		if err != nil {
			return err
		}
		if _, e := files.Read(filepath.Dir(client), filepath.Base(client), d.config.Limits.MaxArtifactBytes); e != nil {
			fmt.Fprintln(d.out, "파일을 읽을 수 없습니다. 로컬 JSON 파일 경로를 확인해 주세요.")
			continue
		}
		break
	}
	policy := gmail.Policy{BatchSize: min(gmail.DefaultBatchSize, d.config.Limits.MaxMessages), HistoryDays: gmail.DefaultHistoryDays, Retention: "manual", Timezone: d.config.Timezone}
	for {
		query, err = d.ask("어떤 메일을 조회할까요? Gmail 검색식을 입력하세요. 예: in:inbox newer_than:7d")
		if err != nil {
			return err
		}
		policy.Query = query
		if e := policy.Validate(d.config); e != nil {
			fmt.Fprintln(d.out, "조회 조건을 확인해 주세요:", e)
			continue
		}
		break
	}
	fmt.Fprintf(d.out, "%s / 조회 조건: %s / 한 번에 최대 %d개. Gmail 읽기 전용 권한을 요청합니다.\n", account, query, policy.BatchSize)
	answer, err := d.ask("이 범위로 브라우저 로그인을 시작할까요? 시작 / 취소")
	if err != nil {
		return err
	}
	if answer != "시작" && !strings.EqualFold(answer, "yes") {
		fmt.Fprintln(d.out, "로그인을 시작하지 않았습니다.")
		return nil
	}
	return d.startAndWait("gmail.start", onboarding.Request{ID: files.ID(), ClientPath: client, Account: account, Policy: policy})
}

func (d *setupDialogue) startAndWait(operation string, request onboarding.Request) error {
	result, err := d.call(operation, request)
	if err != nil {
		fmt.Fprintln(d.out, "설정을 시작하지 못했습니다:", err)
		return nil
	}
	fmt.Fprintln(d.out, "설정 작업:", request.ID)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(d.config.Limits.LockWaitSeconds)*time.Second)
		defer cancel()
		_, _ = setupCall(ctx, d.root, d.config, "cancel", onboarding.Request{ID: request.ID})
	}()
	ticker := time.NewTicker(onboarding.PollInterval(d.config))
	defer ticker.Stop()
	shown, opened := "", false
	for {
		if result.Message != shown {
			fmt.Fprintln(d.out, result.Message)
			shown = result.Message
		}
		if result.AuthURL != "" && !opened {
			opened = true
			if d.noBrowser {
				fmt.Fprintln(d.out, "로그인 주소:", result.AuthURL)
			} else {
				opener, e := exec.LookPath("open")
				if e == nil {
					e = exec.CommandContext(d.ctx, opener, result.AuthURL).Run()
				}
				if e != nil {
					fmt.Fprintln(d.out, "브라우저를 열지 못했습니다. 이 주소로 로그인하세요:", result.AuthURL)
				}
			}
		}
		switch result.Status {
		case "connected":
			fmt.Fprintln(d.out, "연결 ID:", result.ConnectionID)
			return nil
		case "failed", "cancelled", "interrupted":
			return nil
		}
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		case line, ok := <-d.lines:
			if !ok {
				return io.EOF
			}
			if line.err != nil {
				return line.err
			}
			switch strings.ToLower(line.text) {
			case "종료", "그만", "quit", "exit":
				return io.EOF
			case "취소", "뒤로", "cancel", "back":
				if _, err = d.call("cancel", onboarding.Request{ID: request.ID}); err != nil {
					return err
				}
				fmt.Fprintln(d.out, "취소를 요청했습니다. 정리 결과를 기다립니다.")
			default:
				fmt.Fprintln(d.out, "브라우저에서 로그인하거나 ‘취소’를 입력하세요.")
			}
		case <-ticker.C:
			result, err = d.call("status", onboarding.Request{ID: request.ID})
			if err != nil {
				return err
			}
		}
	}
}
