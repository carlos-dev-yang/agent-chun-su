package reception

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/conversation"
	"chunsu/internal/onboarding"
	"chunsu/internal/store"
)

type HostResult struct {
	Status string `json:"status"`
	Detail any    `json:"detail,omitempty"`
}

// Local callbacks contain user interaction only. Common admission, authority
// checks and result projection do not depend on Cobra or a particular UI.
type Host struct {
	Root           string
	Config         config.Config
	GmailSetup     func(context.Context) (HostResult, error)
	DisplayReport  func(context.Context, []byte) error
	GuideInstalled func(onboarding.Result)
}

func (h Host) call(ctx context.Context, request control.Request, result any) error {
	data, handled, err := control.Call(ctx, h.Root, time.Duration(h.Config.Limits.LockWaitSeconds)*time.Second, h.Config.Limits.MaxArtifactBytes, request)
	if err != nil {
		return err
	}
	if !handled {
		return errors.New("호스트가 실행 중이지 않습니다")
	}
	return json.Unmarshal(data, result)
}

func (h Host) Dispatch(ctx context.Context, channel string, action conversation.Action, request string, known map[string]bool) (HostResult, error) {
	if err := conversation.Validate(conversation.Reply{Message: "dispatch", Action: action}); err != nil {
		return HostResult{}, err
	}
	if !allowed(channel, action.Name) {
		return HostResult{}, errors.New("reception channel does not permit this action")
	}
	switch action.Name {
	case conversation.RuntimeStatus:
		var status struct {
			ActiveJob   string `json:"active_job"`
			Owner       string `json:"owner"`
			QueuePaused bool   `json:"queue_paused"`
		}
		err := h.call(ctx, control.Request{Operation: "status"}, &status)
		return HostResult{Status: "observed", Detail: status}, err
	case conversation.ReadGuide:
		b, err := conversation.Guide(action.Service)
		if err != nil {
			return HostResult{}, err
		}
		if int64(len(b)) > h.Config.Limits.MaxSourceBytes {
			return HostResult{}, errors.New("매뉴얼이 대화 입력 한도를 초과합니다")
		}
		return HostResult{Status: "guide_read", Detail: string(b)}, nil
	case conversation.InstallGuide:
		b, _ := json.Marshal(onboarding.Request{Service: action.Service})
		var result onboarding.Result
		if err := h.call(ctx, control.Request{Operation: onboarding.Prefix + "prepare", Input: b}, &result); err != nil {
			return HostResult{}, err
		}
		if channel == Local && h.GuideInstalled != nil {
			h.GuideInstalled(result)
		}
		return HostResult{Status: result.Status, Detail: map[string]string{"service": action.Service, "pack_digest": result.PackDigest, "message": result.Message}}, nil
	case conversation.GmailSetup:
		if channel != Local || h.GmailSetup == nil {
			return HostResult{}, errors.New("Gmail 인증은 로컬 입력 화면에서 진행해 주세요")
		}
		return h.GmailSetup(ctx)
	case conversation.ListJobs:
		s, err := store.OpenReadOnly(ctx, h.Root)
		if err != nil {
			return HostResult{}, err
		}
		defer s.Close()
		jobs, err := s.Jobs(ctx)
		if err != nil {
			return HostResult{}, err
		}
		type summary struct {
			ID        string `json:"id"`
			Workgroup string `json:"workgroup"`
			Status    string `json:"status"`
			CreatedAt int64  `json:"created_at"`
		}
		items := []summary{}
		for i, job := range jobs {
			if i >= h.Config.Limits.MaxMessages {
				break
			}
			items = append(items, summary{job.ID, job.Workgroup, job.Status, job.CreatedAt})
			known[job.ID] = true
		}
		return HostResult{Status: "observed", Detail: map[string]any{"jobs": items, "total": len(jobs)}}, nil
	case conversation.DelegateJob, conversation.ShowReport:
		if !known[action.Reference] && !strings.Contains(request, action.Reference) {
			return HostResult{}, errors.New("먼저 작업 목록에서 확인한 ID나 사용자가 지정한 ID를 선택해야 합니다")
		}
		if action.Name == conversation.DelegateJob {
			var job store.Job
			if err := h.call(ctx, control.Request{Operation: "delegate", JobID: action.Reference, Answer: request}, &job); err != nil {
				return HostResult{}, err
			}
			known[job.ID] = true
			return HostResult{Status: "queued", Detail: map[string]string{"job_id": job.ID, "workgroup": job.Workgroup, "state": job.Status, "note": "별도 업무로 접수했습니다. 실행 중인 worker가 처리하며, 아직 결과가 생성된 것은 아닙니다."}}, nil
		}
		if channel != Local || h.DisplayReport == nil {
			return HostResult{}, errors.New("이 채널에는 보고서 본문 표시 권한이 없습니다")
		}
		s, err := store.OpenReadOnly(ctx, h.Root)
		if err != nil {
			return HostResult{}, err
		}
		defer s.Close()
		job, err := s.Job(ctx, action.Reference)
		if err != nil {
			return HostResult{}, err
		}
		artifacts, err := s.Artifacts(ctx, job.ID)
		if err != nil {
			return HostResult{}, err
		}
		for i := len(artifacts) - 1; i >= 0; i-- {
			artifact := artifacts[i]
			if artifact.Kind != "report_markdown" || artifact.AttemptID != job.CurrentAttempt {
				continue
			}
			b, err := s.ReadArtifact(artifact, h.Config.Limits.MaxArtifactBytes)
			if err != nil {
				return HostResult{}, err
			}
			if err = h.DisplayReport(ctx, b); err != nil {
				return HostResult{}, err
			}
			return HostResult{Status: "displayed_locally", Detail: "검증된 보존 보고서를 사용자에게 표시했습니다. 본문은 접수 AI에게 전달하지 않았습니다. 내용을 읽거나 요약했다고 주장하지 마세요."}, nil
		}
		return HostResult{}, errors.New("보존된 보고서가 없습니다")
	default:
		return HostResult{}, errors.New("지원되지 않는 접수 작업입니다")
	}
}
