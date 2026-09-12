// Package conversation defines the host-owned action contract for natural chat.
package conversation

import (
	"encoding/json"
	"errors"
	"strings"

	"chunsu/internal/features"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/onboarding"
	setupskills "chunsu/setup-skills"
)

const MaxActionsPerTurn = 4
const None = "none"
const RuntimeStatus = "runtime_status"
const ReadGuide = "read_guide"
const InstallGuide = "install_guide"
const GmailSetup = "gmail_setup"
const ListJobs = "list_jobs"
const ShowReport = "show_report"
const DelegateJob = "delegate_job"
const ListErrors = "list_errors"
const AcknowledgeError = "acknowledge_error"
const PauseQueue = "pause_queue"
const ResumeQueue = "resume_queue"
const CancelJob = "cancel_job"
const RetryJob = "retry_job"
const ListFeatures = "list_features"
const InstallFeature = "install_feature"
const StartWorker = "start_worker"
const StopWorker = "stop_worker"

type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Service     bool   `json:"service_required"`
	Reference   bool   `json:"reference_required"`
}

func Capabilities() []Capability {
	return []Capability{
		{StartWorker, "사용자가 요청하면 채팅이 소유한 내부 worker의 지속 실행을 요청한다. 큐 중지와 자료 접근 권한은 보존한다. 외부 worker는 그대로 둔다.", false, false},
		{StopWorker, "사용자가 요청하면 채팅이 소유한 내부 worker를 중지한다. 진행 중 실행은 중단될 수 있으며 대화와 관리 명령은 유지한다.", false, false},
		{ListErrors, "누적 운영 오류와 복구 안내를 조회한다. 원문과 비밀값은 포함하지 않는다.", false, false},
		{AcknowledgeError, "사용자가 검토한 오류 ID의 현재 발생분을 확인 처리한다. 기록을 삭제하거나 해결됐다고 주장하지 않는다.", false, true},
		{PauseQueue, "사용자 요청에 따라 새 업무 실행을 일시 중지한다. 진행 중 업무와 채팅은 유지한다.", false, false},
		{ResumeQueue, "사용자가 요청하면 업무 큐를 재개한다. 계정·자료 권한을 변경하지 않는다.", false, false},
		{CancelJob, "사용자가 취소를 요청한 작업 ID를 취소한다.", false, true},
		{RetryJob, "사용자가 재시도를 명시한 실패 작업 ID를 기존 재시도 규칙으로 다시 접수한다. 불명확한 작업을 자동 재시도하지 않는다.", false, true},
		{ListFeatures, "설치 가능한 내장 업무 모듈과 설정 안내를 구분해 조회한다.", false, false},
		{InstallFeature, "사용자가 선택한 기능 ID를 service에 넣어 내장 업무 모듈 또는 안내 자료를 설치한다. 기존 활성 기준과 권한은 보존한다.", true, false},
		{None, "사용자에게 답변만 한다. 텍스트 초안·설명·추가 질문 가능.", false, false},
		{RuntimeStatus, "현재 춘수 실행 상태를 조회한다. 계정과 메일을 조회하지 않는다.", false, false},
		{ReadGuide, "선택 서비스의 번들 설치 매뉴얼을 읽는다. 설치하거나 계정에 접속하지 않는다.", true, false},
		{InstallGuide, "선택 서비스의 설정 자료를 로컬에 설치한다. 외부 서비스 로그인/설치가 아니다.", true, false},
		{GmailSetup, "Gmail 연결/확인을 위한 로컬 질문과 인증 흐름을 시작한다. 비밀 입력은 AI에 전달하지 않는다.", false, false},
		{ListJobs, "기존 작업의 ID·종류·상태를 조회한다. 입력과 보고서 본문은 포함하지 않는다.", false, false},
		{ShowReport, "선택 작업의 보존된 보고서를 검증하여 사용자 터미널에 표시한다. 본문은 AI로 보내지 않는다.", false, true},
		{DelegateJob, "사용자가 재처리를 요청한 보존 작업의 동일 입력을 별도 업무로 접수한다. 현재 활성 스킬로 worker가 실행하며, 접수 AI는 원문·계정·실행 도구에 접근하지 않는다. 접수만으로 완료를 주장하지 않는다.", false, true},
	}
}

type Action struct {
	Name      string `json:"name"`
	Service   string `json:"service"`
	Reference string `json:"reference"`
}
type Reply struct {
	Message string `json:"message"`
	Action  Action `json:"action"`
}
type Event struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func Skill() ([]byte, error) { return setupskills.Assets.ReadFile("converse/SKILL.md") }

func Validate(reply Reply) error {
	if strings.TrimSpace(reply.Message) == "" {
		return errors.New("empty chat reply")
	}
	for _, capability := range Capabilities() {
		if capability.Name != reply.Action.Name {
			continue
		}
		if capability.Reference {
			if !files.ValidID(reply.Action.Reference) {
				return errors.New("invalid chat reference")
			}
		} else if reply.Action.Reference != "" {
			return errors.New("unexpected chat reference")
		}
		if !capability.Service {
			if reply.Action.Service != "" {
				return errors.New("unexpected chat service")
			}
			return nil
		}
		if reply.Action.Name == InstallFeature {
			_, err := features.Lookup(reply.Action.Service)
			return err
		}
		services, err := onboarding.Services()
		if err != nil {
			return err
		}
		for _, service := range services {
			if service.ID == reply.Action.Service {
				return nil
			}
		}
		return errors.New("unsupported chat service")
	}
	return errors.New("unsupported chat action")
}

func Decode(data []byte) (Reply, error) {
	var reply Reply
	if err := mail.Decode(data, &reply); err != nil {
		return reply, err
	}
	return reply, Validate(reply)
}

func Schema() ([]byte, error) {
	return SchemaFor(Capabilities())
}

func SchemaFor(capabilities []Capability) ([]byte, error) {
	names := []string{}
	for _, c := range capabilities {
		names = append(names, c.Name)
	}
	return json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"message", "action"},
		"properties": map[string]any{
			"message": map[string]any{"type": "string"},
			"action": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "service", "reference"},
				"properties": map[string]any{"name": map[string]any{"type": "string", "enum": names}, "service": map[string]any{"type": "string"}, "reference": map[string]any{"type": "string"}}},
		},
	})
}

func Prompt(history []Event, limit int64) ([]byte, error) {
	return PromptFor(history, limit, Capabilities(), "local_terminal")
}

func PromptFor(history []Event, limit int64, capabilities []Capability, channel string) ([]byte, error) {
	skill, err := Skill()
	if err != nil {
		return nil, err
	}
	services, err := onboarding.Services()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(struct {
		Channel      string               `json:"channel"`
		Capabilities []Capability         `json:"capabilities"`
		Services     []onboarding.Service `json:"services"`
		History      []Event              `json:"history"`
	}{channel, capabilities, services, history})
	if err != nil {
		return nil, err
	}
	prompt := append(append(skill, []byte("\n\n호스트가 제공하는 현재 기능과 대화 이력(JSON):\n")...), b...)
	if int64(len(prompt)) > limit {
		return nil, errors.New("대화가 길어졌습니다. /새대화로 대화를 새로 시작해 주세요. 이전 지시를 임의로 생략하지 않았습니다.")
	}
	return prompt, nil
}

func Guide(serviceID string) ([]byte, error) {
	services, err := onboarding.Services()
	if err != nil {
		return nil, err
	}
	for _, service := range services {
		if service.ID == serviceID {
			return setupskills.Assets.ReadFile("connect-services/" + service.Manual)
		}
	}
	return nil, errors.New("unsupported service guide")
}
