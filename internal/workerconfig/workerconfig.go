// Package workerconfig defines the safe task-worker configuration boundary.
package workerconfig

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/runtimeenv"
)

const MaxModelLength = 128

type View struct {
	Configured       bool     `json:"configured"`
	Driver           string   `json:"driver"`
	Model            string   `json:"model"`
	Environment      string   `json:"environment"`
	AvailableSources []string `json:"available_sources"`
}

type Result struct {
	Message  string `json:"message"`
	Settings View   `json:"settings"`
}

// ValidateModel accepts a bounded model identifier that is safe to pass as one
// process argument. It deliberately does not decide which models are supported.
func ValidateModel(value string) error {
	if value == "" || len(value) > MaxModelLength {
		return errors.New("model identifier is missing or too long")
	}
	if strings.HasPrefix(value, "-") {
		return errors.New("model identifier cannot start with an option")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return errors.New("model identifier contains unsupported characters")
		}
	}
	return nil
}

// ValidateExecutor verifies the local execution identity without probing the
// executable or making provider traffic.
func ValidateExecutor(selected config.Executor) error {
	if err := ValidateModel(selected.Model); err != nil {
		return err
	}
	if selected.Kind == "" || selected.Path == "" || !filepath.IsAbs(selected.Path) {
		return errors.New("task worker execution identity is incomplete")
	}
	if _, err := executor.Select(selected.Kind); err != nil {
		return errors.New("task worker execution identity is unsupported")
	}
	if _, err := runtimeenv.Select(selected.Environment); err != nil {
		return errors.New("task worker execution identity is unsupported")
	}
	return nil
}

// Project is the sole safe projection of persisted task-worker configuration.
func Project(c config.Config) View {
	available := make([]string, 0, 2)
	if c.Routes.Reception != nil && ValidateExecutor(*c.Routes.Reception) == nil {
		available = append(available, config.RoleReception)
	}
	if c.Routes.Review != nil && ValidateExecutor(*c.Routes.Review) == nil {
		available = append(available, config.RoleReview)
	}
	selected := c.Executor
	return View{Configured: ValidateExecutor(selected) == nil, Driver: selected.Kind, Model: selected.Model, Environment: selected.Environment, AvailableSources: available}
}
