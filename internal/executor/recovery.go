package executor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/mail"
	"chunsu/internal/platform"
)

// Reconcile is called by the controller owner. It never kills a process solely
// by PID, and an interrupted start without an identity requires inspection.
func Reconcile(ctx context.Context, root, path string, limits config.Limits) error {
	b, err := files.Read(root, path, limits.MaxArtifactBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var record ProcessRecord
	if err = mail.Decode(b, &record); err != nil {
		return err
	}
	switch record.State {
	case "not_started", "exited":
		return nil
	case "starting":
		return errors.New("executor start was interrupted before identity was recorded; inspect surviving processes before resuming")
	case "running":
	default:
		return errors.New("unknown executor process state")
	}
	cleanup, cancel := context.WithTimeout(ctx, time.Duration(limits.LockWaitSeconds)*time.Second)
	defer cancel()
	if err = platform.ReconcileProcess(cleanup, record.Identity); err != nil {
		return err
	}
	record.State = "exited"
	b, err = json.Marshal(record)
	if err != nil {
		return err
	}
	return files.Write(root, path, b, true)
}
