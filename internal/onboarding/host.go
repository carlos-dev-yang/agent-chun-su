// Package onboarding implements fixed host setup operations, separate from AI jobs.
package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/files"
	"chunsu/internal/gmail"
	"chunsu/internal/mail"
	setupskills "chunsu/setup-skills"
)

const Prefix = "setup."
const packRoot = "connect-services"
const taskDirectory = "state/setup-tasks"

type Service struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Manual  string   `json:"manual"`
	Status  string   `json:"chunsu_status"`
	Aliases []string `json:"aliases,omitempty"`
}

func Services() ([]Service, error) {
	b, err := setupskills.Assets.ReadFile(packRoot + "/catalog.json")
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Services []Service `json:"services"`
	}
	err = json.Unmarshal(b, &catalog)
	return catalog.Services, err
}

type Request struct {
	ID           string       `json:"id,omitempty"`
	Service      string       `json:"service,omitempty"`
	ClientPath   string       `json:"client_path,omitempty"`
	Account      string       `json:"account,omitempty"`
	ConnectionID string       `json:"connection_id,omitempty"`
	Policy       gmail.Policy `json:"policy,omitempty"`
}

type Result struct {
	ID           string `json:"id,omitempty"`
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	ManualPath   string `json:"manual_path,omitempty"`
	PackDigest   string `json:"pack_digest,omitempty"`
	AuthURL      string `json:"auth_url,omitempty"` // Memory and private management channel only.
	ConnectionID string `json:"connection_id,omitempty"`
}

type Host struct {
	Root   string
	Config config.Config
	mu     sync.Mutex
	active *Result
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed bool
}

func (h *Host) Close() {
	h.mu.Lock()
	h.closed = true
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Unlock()
	h.wg.Wait()
}

func (h *Host) Handle(ctx context.Context, req control.Request) (any, error) {
	var input Request
	if len(req.Input) != 0 {
		if err := mail.Decode(req.Input, &input); err != nil {
			return nil, errors.New("invalid setup request")
		}
	}
	switch req.Operation {
	case Prefix + "prepare":
		return h.prepare(input.Service)
	case Prefix + "gmail.start":
		return h.start(ctx, input, false)
	case Prefix + "gmail.check":
		return h.start(ctx, input, true)
	case Prefix + "status", Prefix + "cancel":
		h.mu.Lock()
		defer h.mu.Unlock()
		if !files.ValidID(input.ID) {
			return nil, errors.New("invalid setup task ID")
		}
		if h.active != nil && h.active.ID == input.ID {
			if req.Operation == Prefix+"cancel" && h.cancel != nil {
				h.cancel()
			}
			return *h.active, nil
		}
		b, err := files.Read(h.Root, taskPath(input.ID), h.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return nil, err
		}
		var result Result
		if err = mail.Decode(b, &result); err != nil {
			return nil, err
		}
		if result.Status == "running" || result.Status == "awaiting_auth" {
			result.Status, result.Message = "interrupted", "설정 프로세스가 중단되었습니다. 기존 연결 목록을 확인한 뒤 다시 시작하세요."
		}
		return result, nil
	default:
		return nil, errors.New("unsupported setup operation")
	}
}

func taskPath(id string) string { return filepath.Join(taskDirectory, id+".json") }

func (h *Host) save(result Result, replace bool) error {
	result.AuthURL = ""
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return files.Write(h.Root, taskPath(result.ID), b, replace)
}

func (h *Host) start(ctx context.Context, input Request, checkOnly bool) (Result, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return Result{}, errors.New("setup host is stopping")
	}
	if !files.ValidID(input.ID) {
		return Result{}, errors.New("invalid setup task ID")
	}
	if h.active != nil && h.cancel != nil {
		return Result{}, errors.New("다른 인증이 진행 중입니다. 완료하거나 취소한 뒤 다시 시작하세요.")
	}
	if _, err := files.Read(h.Root, taskPath(input.ID), h.Config.Limits.MaxArtifactBytes); !errors.Is(err, os.ErrNotExist) {
		return Result{}, errors.New("설정 요청 ID가 이미 존재하거나 확인할 수 없습니다. 상태를 먼저 조회하세요.")
	}
	var clientData []byte
	var existing gmail.Connection
	var err error
	if checkOnly {
		existing, err = gmail.LoadConnection(h.Root, input.ConnectionID, h.Config)
		if err != nil {
			return Result{}, err
		}
	} else {
		if err := gmail.CheckAccount(input.Account, input.Account); err != nil {
			return Result{}, err
		}
		if err := input.Policy.Validate(h.Config); err != nil {
			return Result{}, err
		}
		if !filepath.IsAbs(input.ClientPath) {
			return Result{}, errors.New("OAuth client 파일의 절대 경로가 필요합니다")
		}
		clientData, err = files.Read(filepath.Dir(input.ClientPath), filepath.Base(input.ClientPath), h.Config.Limits.MaxArtifactBytes)
		if err != nil {
			return Result{}, errors.New("OAuth client 파일을 읽을 수 없습니다. 일반 파일의 경로와 크기를 확인하세요.")
		}
	}
	result := Result{ID: input.ID, Status: "running", Message: "Gmail 읽기 전용 연결을 준비하고 있습니다."}
	if err = h.save(result, false); err != nil {
		return Result{}, err
	}
	taskCtx, cancel := context.WithCancel(ctx)
	h.active, h.cancel = &result, cancel
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer cancel()
		var connection gmail.Connection
		var connectErr error
		if checkOnly {
			checkCtx, done := context.WithTimeout(taskCtx, time.Duration(gmail.DefaultHTTPTimeoutSeconds)*time.Second)
			client, e := gmail.OpenClient(checkCtx, existing, h.Config)
			if e == nil {
				var account string
				account, e = client.Profile(checkCtx)
				if e == nil {
					e = gmail.CheckAccount(existing.Account, account)
				}
			}
			done()
			connectErr = e
			if e == nil {
				connection = existing
			}
		} else {
			connection, connectErr = gmail.Connect(taskCtx, h.Root, h.Config, clientData, input.Account, input.Policy, nil, func(authURL string) error {
				h.mu.Lock()
				defer h.mu.Unlock()
				h.active.Status, h.active.AuthURL = "awaiting_auth", authURL
				h.active.Message = "브라우저에서 Google 로그인을 완료하세요. 취소할 수 있습니다."
				return h.save(*h.active, true)
			})
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		h.active.AuthURL = ""
		h.active.ConnectionID = connection.ID
		switch {
		case connection.ID != "":
			h.active.Status = "connected"
			h.active.Message = "계정과 읽기 권한을 확인하고 연결을 저장했습니다. 메일 수집과 AI 공개는 별도 설정입니다."
			if checkOnly {
				h.active.Message = "기존 계정과 읽기 권한을 확인했습니다. 다시 로그인하지 않고 이 연결을 사용할 수 있습니다."
			}
		case taskCtx.Err() != nil:
			h.active.Status, h.active.Message = "cancelled", "인증을 취소했습니다. 연결 완료로 표시하지 않습니다."
		case connectErr != nil:
			h.active.Status, h.active.Message = "failed", connectErr.Error()
		default:
			h.active.Status, h.active.Message = "failed", "연결 결과를 확인할 수 없습니다."
		}
		if err := h.save(*h.active, true); err != nil {
			h.active.Message += " 상태 기록 저장에 실패했습니다. 연결 목록을 확인하세요."
		}
		h.cancel = nil
	}()
	return result, nil
}

func (h *Host) prepare(serviceID string) (Result, error) {
	services, err := Services()
	if err != nil {
		return Result{}, err
	}
	var selected *Service
	for i := range services {
		if services[i].ID == serviceID {
			selected = &services[i]
		}
	}
	if selected == nil {
		return Result{}, errors.New("지원 목록에서 서비스를 선택하세요")
	}
	content := map[string][]byte{}
	var manifest strings.Builder
	err = fs.WalkDir(setupskills.Assets, packRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		b, e := setupskills.Assets.ReadFile(path)
		if e != nil {
			return e
		}
		content[path] = b
		fmt.Fprintf(&manifest, "%s %s\n", path, files.Digest(b))
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	digest := files.Digest([]byte(manifest.String()))
	destination := filepath.Join("setup-skills", digest)
	for path, data := range content {
		target := filepath.Join(destination, path)
		if err = files.Write(h.Root, target, data, false); err != nil {
			existing, readErr := files.Read(h.Root, target, h.Config.Limits.MaxArtifactBytes)
			if readErr != nil || files.Digest(existing) != files.Digest(data) {
				return Result{}, errors.New("기존 설치 자료가 다릅니다. 파일을 덮어쓰지 않았습니다.")
			}
		}
	}
	return Result{Status: "guidance_installed", PackDigest: digest,
		ManualPath: filepath.Join(h.Root, destination, packRoot, selected.Manual),
		Message:    selected.Name + " 설정 자료를 로컬에 설치했습니다. " + selected.Status}, nil
}

func PollInterval(c config.Config) time.Duration {
	return time.Duration(c.Limits.PollSeconds) * time.Second
}
