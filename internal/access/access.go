package access

import (
	"chunsu/internal/audit"
	"chunsu/internal/files"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const BlockPath = "state/execution-blocked.json"
const BlockedMessage = "AI execution is suspended by the data-home owner; review access status and explicitly resume before new execution"

// Watch cancels an already-running agent when the owner revokes execution.
// All roles use the configured poll interval and their driver's normal cleanup.
func Watch(parent context.Context, root string, interval time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if interval <= 0 {
		cancel()
		return ctx, cancel
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if Check(root) != nil {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ctx, cancel
}

func Check(root string) error {
	_, err := os.Lstat(filepath.Join(root, BlockPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New(BlockedMessage)
}

func Block(root string) error {
	data, err := json.Marshal(map[string]any{"version": 1, "at": time.Now().UTC().Format(time.RFC3339Nano), "os_user_id": os.Geteuid()})
	if err != nil {
		return err
	}
	return files.Write(root, BlockPath, data, true)
}

func Resume(root string) error {
	path := filepath.Join(root, BlockPath)
	if err := files.RequirePrivateFile(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := audit.Record(root, "execution.resume_requested", "", ""); err != nil {
		return err
	}
	return os.Remove(path)
}
