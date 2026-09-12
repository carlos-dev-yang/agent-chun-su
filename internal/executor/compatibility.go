package executor

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"chunsu/internal/config"
)

// This additional version is validated only for the tool-free reception route
// on macOS/arm64. It does not extend task or review disclosure authorization.
const TestedReceptionVersion = "codex-cli 0.154.0-alpha.6.2"

// CompatibilityError contains only a local prerequisite diagnostic, so a
// transport can distinguish it from private provider or host-action errors.
type CompatibilityError struct{ detail string }

func (e *CompatibilityError) Error() string { return e.detail }

func codexCompatibility(role, version, model string) error {
	versions := []string{TestedVersion}
	if role == config.RoleReception && runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		versions = append(versions, TestedReceptionVersion)
	}
	if model != TestedModel {
		return &CompatibilityError{detail: fmt.Sprintf("AI 실행 모델 %q은 검증되지 않았습니다. 지원 모델: %s. 모델 변경은 별도 실행 경계 재검증이 필요합니다.", model, TestedModel)}
	}
	if !slices.Contains(versions, version) {
		return &CompatibilityError{detail: fmt.Sprintf("설치된 AI 실행기 %q은 검증되지 않았습니다. %s 경로 지원 버전: %s. 실행기 변경은 별도 실행 경계 재검증이 필요합니다.", version, role, strings.Join(versions, ", "))}
	}
	return nil
}
