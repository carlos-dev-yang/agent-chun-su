package reception

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"chunsu/internal/config"
	"chunsu/internal/executor"
	"chunsu/internal/files"
)

func (s *Session) Recover(ctx context.Context) error {
	if !files.ValidID(s.ID) {
		return errors.New("invalid local reception session")
	}
	directory := filepath.Join("chat", s.ID)
	entries, err := os.ReadDir(filepath.Join(s.Root, directory))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && files.ValidID(entry.Name()) {
			if err = executor.Reconcile(ctx, s.Root, filepath.Join(directory, entry.Name(), executor.ProcessFile), s.Config.Limits); err != nil {
				return err
			}
		}
	}
	return nil
}

type ActionError struct{ Cause error }

func (e *ActionError) Error() string { return "reception host action failed" }
func (e *ActionError) Unwrap() error { return e.Cause }

func ErrorCode(err error, c config.Config) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "model_timeout"
	}
	if errors.Is(err, ErrUncertain) {
		return "execution_uncertain"
	}
	var action *ActionError
	if errors.As(err, &action) {
		return "host_failed"
	}
	var compatibility *executor.CompatibilityError
	if errors.As(err, &compatibility) {
		return "model_compatibility"
	}
	selected := c.ExecutorFor(config.RoleReception)
	if selected.Kind == "" || selected.Path == "" || selected.Model == "" {
		return "model_not_configured"
	}
	return "model_failed"
}
