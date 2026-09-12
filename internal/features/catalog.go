// Package features lists compiled, host-owned installation operations. Selecting
// a catalog entry never downloads code or grants source/model permissions.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/onboarding"
	"chunsu/internal/workgroup"
)

type Feature struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Requirements string `json:"requirements"`
}

func Catalog() ([]Feature, error) {
	items := []Feature{}
	for _, definition := range workgroup.Definitions() {
		items = append(items, Feature{ID: "workflow-" + definition.ID, Kind: "workflow", Name: definition.ID, Requirements: "번들 스킬과 기준 설치. 기존 활성 기준을 보존합니다. 계정 연결·자료 접근 승인·worker 실행은 별도 상태입니다."})
	}
	services, err := onboarding.Services()
	if err != nil {
		return nil, err
	}
	for _, service := range services {
		items = append(items, Feature{ID: "guide-" + service.ID, Kind: "guide", Name: service.Name, Requirements: "설정 매뉴얼 설치. 외부 프로그램이나 커넥터 설치 완료를 뜻하지 않습니다. " + service.Status})
	}
	return items, nil
}

func Lookup(id string) (Feature, error) {
	items, err := Catalog()
	if err != nil {
		return Feature{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return Feature{}, errors.New("unsupported feature; select an ID from /features")
}

// Install is called by the controller writer, never directly by reception AI.
func Install(ctx context.Context, root, id string, c config.Config) (any, error) {
	item, err := Lookup(id)
	if err != nil {
		return nil, err
	}
	if item.Kind == "workflow" {
		workgroupID := strings.TrimPrefix(id, "workflow-")
		if err = workgroup.InstallFor(root, workgroupID, c.Limits.MaxArtifactBytes); err != nil {
			return nil, err
		}
		_, digest, err := workgroup.ActiveFor(root, workgroupID, c.Limits.MaxArtifactBytes)
		return map[string]string{"feature": id, "status": "installed", "active_digest": digest, "next": item.Requirements}, err
	}
	host := &onboarding.Host{Root: root, Config: c}
	defer host.Close()
	input, _ := json.Marshal(onboarding.Request{Service: strings.TrimPrefix(id, "guide-")})
	value, err := host.Handle(ctx, control.Request{Operation: onboarding.Prefix + "prepare", Input: input})
	if err != nil {
		return nil, err
	}
	result := value.(onboarding.Result)
	return map[string]string{"feature": id, "status": "guide_installed", "pack_digest": result.PackDigest, "next": item.Requirements}, nil
}
