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
const TestedStructuredVersion = "codex-cli 0.158.0-alpha.2.1"
const TestedStructuredStableVersion = "codex-cli 0.160.0"

// This exact host/CLI/model/effort combination passed a synthetic tool-free
// boundary check. It does not validate the native-MCP report path or Linux.
func structuredBoundaryValidated(role, version string, selected config.Executor) bool {
	if role == config.RoleTask || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || selected.Kind != "codex" {
		return false
	}
	if version == TestedStructuredStableVersion {
		// These requested role/model/effort tuples were actually exercised with
		// agents.enabled=false in addition to the disabled native tool host.
		// Collection remains a host operation; native report and Linux paths are
		// not covered by this structured-only evidence.
		switch role {
		case config.RoleReception:
			return selected.Model == SolModel && selected.ReasoningEffort == "medium"
		case config.RoleRefinement:
			return selected.Model == LunaModel && selected.ReasoningEffort == "xhigh"
		case config.RoleSynthesis, config.RoleReview:
			return selected.Model == SolModel && selected.ReasoningEffort == "xhigh"
		}
		return false
	}
	if version != TestedStructuredVersion {
		return false
	}
	switch selected.Model {
	case SolModel:
		return selected.ReasoningEffort == "medium" || selected.ReasoningEffort == "xhigh"
	case LunaModel:
		return selected.ReasoningEffort == "xhigh"
	}
	return false
}

func codexSelectedCompatibility(role, version string, selected config.Executor) error {
	if structuredBoundaryValidated(role, version, selected) {
		return nil
	}
	return codexCompatibility(role, version, selected.Model)
}

// CompatibilityError contains only a local prerequisite diagnostic, so a
// transport can distinguish it from private provider or host-action errors.
type CompatibilityError struct{ detail string }

func (e *CompatibilityError) Error() string { return e.detail }

func codexCompatibility(role, version, model string) error {
	versions := []string{TestedVersion}
	if role == config.RoleReception && runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		versions = append(versions, TestedReceptionVersion)
	}
	if _, ok := codexModelPreset(model); !ok {
		models := CodexModels().Models
		ids := make([]string, 0, len(models))
		for _, preset := range models {
			ids = append(ids, preset.ID)
		}
		return &CompatibilityError{detail: fmt.Sprintf("AI 실행 모델 %q은 지원되지 않습니다. 선택 가능 모델: %s.", model, strings.Join(ids, ", "))}
	}
	if !slices.Contains(versions, version) {
		return &CompatibilityError{detail: fmt.Sprintf("설치된 AI 실행기 %q은 검증되지 않았습니다. %s 경로 지원 버전: %s. 실행기 변경은 별도 실행 경계 재검증이 필요합니다.", version, role, strings.Join(versions, ", "))}
	}
	return nil
}
