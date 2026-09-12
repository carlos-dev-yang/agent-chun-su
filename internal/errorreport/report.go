// Package errorreport stores bounded operational diagnostics independently of
// the SQLite controller. Only catalogued messages and opaque correlations cross
// this boundary; error strings and conversation contents are never accepted.
package errorreport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/platform"
)

const Directory = "state/errors"
const Version = 1

type Diagnostic struct {
	Component string `json:"component"`
	Summary   string `json:"summary"`
	Recovery  string `json:"recovery"`
}

var catalog = map[string]Diagnostic{
	"poll_failed":             {"telegram", "메시지 수신 연결 실패", "저장된 수신 위치에서 자동 재연결합니다. 반복되면 호스트 네트워크를 확인하세요."},
	"authentication_failed":   {"telegram", "Telegram 인증 또는 접근 거부", "수신 재시도는 유지됩니다. 호스트에서 봇 토큰과 접근 권한을 복구하세요."},
	"poll_conflict":           {"telegram", "다른 수신기 또는 웹훅과 충돌", "같은 봇을 사용하는 다른 수신기나 웹훅을 확인하세요. 기존 웹훅은 자동 삭제하지 않습니다."},
	"send_unconfirmed":        {"telegram", "답장 전송 완료를 확인하지 못함", "중복 전송을 피하려고 자동 재전송하지 않습니다. /status 또는 /jobs로 처리 결과를 확인하세요."},
	"receipt_failed":          {"telegram", "메시지 처리 상태 저장 실패", "해당 요청 실행을 보류합니다. 호스트의 데이터 디렉터리 권한과 저장 공간을 확인하세요."},
	"model_failed":            {"reception", "AI 답변 생성 실패", "수신과 고정 명령은 계속 동작합니다. /reset 또는 호스트의 chunsu doctor로 실행기와 인증을 확인하세요."},
	"execution_uncertain":     {"reception", "실행 종료 또는 작업 결과 확인 필요", "해당 작업을 자동 재실행하지 않습니다. /jobs와 호스트의 복구 상태를 확인하세요."},
	"host_failed":             {"controller", "내부 관리 작업 실패", "수신은 유지됩니다. /status와 /errors로 상태를 확인하고 필요하면 호스트에서 복구하세요."},
	"turn_panicked":           {"reception", "대화 처리 중 내부 오류", "해당 대화를 중단했습니다. 오류 ID와 설치 버전을 사용해 점검하세요."},
	"startup_failed":          {"chat", "채팅 수신기 시작 실패", "감독 프로세스가 재시도합니다. 설정·비밀 저장소·다른 수신기 실행 여부를 확인하세요."},
	"receiver_exited":         {"supervisor", "채팅 수신기 비정상 종료", "감독 프로세스가 수신기를 다시 시작합니다. 중단된 요청은 자동 재실행하지 않습니다."},
	"receiver_stalled":        {"supervisor", "채팅 수신기 응답 확인 시간 초과", "감독 프로세스가 소유한 수신기만 정리하고 다시 시작합니다."},
	"recovery_blocked":        {"supervisor", "이전 실행의 안전한 종료를 확인하지 못함", "중복 실행을 보류합니다. 호스트에서 남은 프로세스와 실행 기록을 확인하세요."},
	"supervisor_state_failed": {"supervisor", "감독 상태 저장 실패", "호스트 데이터 디렉터리 권한과 저장 공간을 복구하세요."},
	"interrupted_request":     {"reception", "재시작 전에 완료되지 않은 요청 발견", "작업 결과가 불명확할 수 있어 자동 재실행하지 않습니다. /jobs로 확인한 후 필요한 요청만 다시 보내세요."},
}

type Correlation struct {
	UpdateID  int64  `json:"update_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	JobID     string `json:"job_id,omitempty"`
}

func (c Correlation) valid() bool {
	return c.UpdateID >= 0 && (c.SessionID == "" || files.ValidID(c.SessionID)) && (c.JobID == "" || files.ValidID(c.JobID))
}

type Occurrence struct {
	At          time.Time   `json:"at"`
	Correlation Correlation `json:"correlation"`
}

type Report struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Code    string `json:"code"`
	Diagnostic
	Count        uint64       `json:"count"`
	Acknowledged uint64       `json:"acknowledged_count"`
	FirstAt      time.Time    `json:"first_at"`
	LastAt       time.Time    `json:"last_at"`
	Recent       []Occurrence `json:"recent"`
}

func ID(code string) string { return files.Digest([]byte("chunsu-error:" + code)) }

func add(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}

// Recorder retains a bounded pending aggregate when persistence is temporarily
// unavailable. A later Flush retries it; Unpersisted is exposed in chat health.
type Recorder struct {
	Root    string
	Limits  config.Limits
	mu      sync.Mutex
	pending map[string]Report
}

func New(root string, limits config.Limits) *Recorder {
	return &Recorder{Root: root, Limits: limits, pending: make(map[string]Report)}
}

func (r *Recorder) Record(ctx context.Context, code string, correlation Correlation) (string, error) {
	diagnostic, ok := catalog[code]
	if !ok || !correlation.valid() {
		return "", errors.New("invalid error report classification or correlation")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	p := r.pending[code]
	if p.Count == 0 {
		p = Report{Version: Version, ID: ID(code), Code: code, Diagnostic: diagnostic, FirstAt: now}
	}
	p.Count = add(p.Count, 1)
	p.LastAt = now
	p.Recent = recent(append(p.Recent, Occurrence{At: now, Correlation: correlation}), r.Limits.MaxMessages)
	r.pending[code] = p
	return p.ID, r.flush(ctx)
}

func recent(items []Occurrence, limit int) []Occurrence {
	if limit < 1 {
		limit = 1
	}
	if len(items) > limit {
		items = append([]Occurrence(nil), items[len(items)-limit:]...)
	}
	return items
}

func lock(ctx context.Context, root string, limits config.Limits) (*platform.Lock, error) {
	if err := files.RequirePrivateDir(root); err != nil {
		return nil, err
	}
	base := filepath.Join(root, Directory)
	for _, path := range []string{filepath.Join(root, "state"), base, filepath.Join(base, "state")} {
		if err := files.PrivateDir(path); err != nil {
			return nil, err
		}
	}
	return platform.Acquire(ctx, base, time.Duration(limits.LockWaitSeconds)*time.Second)
}

func (r *Recorder) Flush(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flush(ctx)
}

func (r *Recorder) Unpersisted() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n uint64
	for _, p := range r.pending {
		n = add(n, p.Count)
	}
	return n
}

func (r *Recorder) flush(ctx context.Context) error {
	if len(r.pending) == 0 {
		return nil
	}
	l, err := lock(ctx, r.Root, r.Limits)
	if err != nil {
		return err
	}
	defer l.Close()
	for code, p := range r.pending {
		old, err := read(r.Root, code, r.Limits)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			p.FirstAt = old.FirstAt
			p.Count = add(old.Count, p.Count)
			p.Acknowledged = old.Acknowledged
			p.Recent = recent(append(old.Recent, p.Recent...), r.Limits.MaxMessages)
		}
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		if int64(len(data)) > r.Limits.MaxArtifactBytes {
			return errors.New("error report exceeds configured artifact limit")
		}
		if err = files.Write(r.Root, filepath.Join(Directory, code+".json"), append(data, '\n'), true); err != nil {
			return err
		}
		delete(r.pending, code)
	}
	return nil
}

func read(root, code string, limits config.Limits) (Report, error) {
	var p Report
	path := filepath.Join(Directory, code+".json")
	if err := files.RequirePrivateFile(filepath.Join(root, path)); err != nil {
		return p, err
	}
	data, err := files.Read(root, path, limits.MaxArtifactBytes)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return Report{}, errors.New("invalid error report")
	}
	if p.Version != Version || p.ID != ID(code) || p.Code != code || p.Acknowledged > p.Count || p.FirstAt.IsZero() || p.LastAt.Before(p.FirstAt) {
		return Report{}, errors.New("invalid error report metadata")
	}
	for _, occurrence := range p.Recent {
		if !occurrence.Correlation.valid() {
			return Report{}, errors.New("invalid error correlation")
		}
	}
	// Text is always regenerated from the trusted catalog, never echoed from disk.
	p.Diagnostic = catalog[code]
	p.Recent = recent(p.Recent, limits.MaxMessages)
	return p, nil
}

func List(root string, limits config.Limits) ([]Report, error) {
	reports := []Report{}
	for code := range catalog {
		p, err := read(root, code, limits)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		reports = append(reports, p)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].LastAt.After(reports[j].LastAt) })
	return reports, nil
}

func Acknowledge(ctx context.Context, root, id string, limits config.Limits) error {
	l, err := lock(ctx, root, limits)
	if err != nil {
		return err
	}
	defer l.Close()
	for code := range catalog {
		if id != ID(code) {
			continue
		}
		p, err := read(root, code, limits)
		if err != nil {
			return err
		}
		p.Acknowledged = p.Count
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		return files.Write(root, filepath.Join(Directory, code+".json"), append(data, '\n'), true)
	}
	return fmt.Errorf("unknown error report ID")
}
