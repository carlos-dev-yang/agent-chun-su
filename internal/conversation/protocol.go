// Package conversation defines the host-owned action contract for natural chat.
package conversation

import (
	"encoding/json"
	"errors"
	"strings"

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

type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Service     bool   `json:"service_required"`
	Reference   bool   `json:"reference_required"`
}

func Capabilities() []Capability {
	return []Capability{
		{None, "사용자에게 답변만 한다. 텍스트 초안·설명·추가 질문 가능.", false, false},
		{RuntimeStatus, "현재 춘수 실행 상태를 조회한다. 계정과 메일을 조회하지 않는다.", false, false},
		{ReadGuide, "선택 서비스의 번들 설치 매뉴얼을 읽는다. 설치하거나 계정에 접속하지 않는다.", true, false},
		{InstallGuide, "선택 서비스의 설정 자료를 로컬에 설치한다. 외부 서비스 로그인/설치가 아니다.", true, false},
		{GmailSetup, "Gmail 연결/확인을 위한 로컬 질문과 인증 흐름을 시작한다. 비밀 입력은 AI에 전달하지 않는다.", false, false},
		{ListJobs, "기존 작업의 ID·종류·상태를 조회한다. 입력과 보고서 본문은 포함하지 않는다.", false, false},
		{ShowReport, "선택 작업의 보존된 보고서를 검증하여 사용자 터미널에 표시한다. 본문은 AI로 보내지 않는다.", false, true},
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
