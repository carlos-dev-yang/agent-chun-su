package executor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"chunsu/internal/runtimeenv"
)

const (
	// TestedModel remains the validated preset used by existing routes and
	// historical evidence.
	TestedModel = "gpt-5.5"
	AstraModel  = "gpt-6-astra"
)

type ModelPreset struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	CodeModeOnly    bool   `json:"-"`
}

type ModelMetadata struct {
	Driver       string        `json:"driver"`
	Version      string        `json:"version"`
	DefaultModel string        `json:"default_model"`
	Models       []ModelPreset `json:"models"`
	Environment  string        `json:"environment"`
}

var codexModelPresets = [...]ModelPreset{
	{ID: TestedModel, Status: "validated"},
	// Validated for the synthetic macOS/arm64 task boundary only.
	{ID: AstraModel, Status: "validated", ReasoningEffort: "low", CodeModeOnly: true},
}

// CodexModels describes the local model policy without contacting a provider.
func CodexModels() ModelMetadata {
	models := make([]ModelPreset, len(codexModelPresets))
	copy(models, codexModelPresets[:])
	return ModelMetadata{
		Driver:       "codex",
		Version:      TestedVersion,
		DefaultModel: AstraModel,
		Models:       models,
		Environment:  runtimeenv.NativeRestricted,
	}
}

func codexModelPreset(id string) (ModelPreset, bool) {
	for _, preset := range codexModelPresets {
		if preset.ID == id {
			return preset, true
		}
	}
	return ModelPreset{}, false
}

func codexReportFeatures(model string) []string {
	preset, ok := codexModelPreset(model)
	if !ok || !preset.CodeModeOnly {
		return nil
	}
	return []string{"--enable", "code_mode_host", "--enable", "code_mode_only"}
}

func codexCodeModeHost(path string) (string, error) {
	executable, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("cannot resolve the Codex executable for the code-mode host check")
	}
	sibling := filepath.Join(filepath.Dir(executable), "codex-code-mode-host")
	if info, err := os.Stat(sibling); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
		return sibling, nil
	}
	if host, err := exec.LookPath("codex-code-mode-host"); err == nil {
		return host, nil
	}
	return "", errors.New("Codex code-mode host is unavailable; install the full Codex distribution including codex-code-mode-host")
}

func codexTaskPrerequisites(path, model string) error {
	preset, ok := codexModelPreset(model)
	if !ok || !preset.CodeModeOnly {
		return nil
	}
	_, err := codexCodeModeHost(path)
	return err
}
