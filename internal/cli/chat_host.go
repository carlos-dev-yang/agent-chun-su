package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"chunsu/internal/chatsupervisor"
	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/errorreport"
	"chunsu/internal/executor"
	"chunsu/internal/platform"
	"chunsu/internal/service"
	"github.com/spf13/cobra"
)

func startOwnedSetupHost(ctx context.Context, cmd *cobra.Command, root string, c config.Config, processPath string) (func(), error) {
	check := func() (bool, error) {
		_, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
		return handled, err
	}
	if handled, err := check(); handled || err != nil {
		return func() {}, err
	}
	if processPath != "" {
		if err := executor.Reconcile(ctx, root, processPath, c.Limits); err != nil {
			return nil, err
		}
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	childCtx, cancel := context.WithCancel(ctx)
	args := []string{"--home", root, "setup", "serve"}
	if processPath != "" {
		enabled, e := service.WorkerEnabled(root)
		if e != nil {
			cancel()
			return nil, e
		}
		if enabled {
			args = []string{"--home", root, "worker"}
		}
	}
	child := exec.CommandContext(childCtx, binary, args...)
	platform.ProcessGroup(child)
	child.Stderr = cmd.ErrOrStderr()
	if processPath != "" {
		child.Stderr = io.Discard
	}
	child.Cancel = func() error { return child.Process.Signal(os.Interrupt) }
	child.WaitDelay = time.Duration(c.Limits.LockWaitSeconds) * time.Second
	if processPath != "" {
		if err = chatsupervisor.SaveProcess(root, processPath, "starting", platform.ProcessIdentity{}); err != nil {
			cancel()
			return nil, err
		}
	}
	if err = child.Start(); err != nil {
		cancel()
		if processPath != "" {
			_ = chatsupervisor.SaveProcess(root, processPath, "not_started", platform.ProcessIdentity{})
		}
		return nil, err
	}
	identity, err := platform.Identify(child.Process.Pid)
	if err == nil && processPath != "" {
		err = chatsupervisor.SaveProcess(root, processPath, "running", identity)
	}
	if err != nil {
		cancel()
		_ = platform.KillGroup(child.Process.Pid)
		_ = child.Wait()
		return nil, err
	}
	done := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = child.Wait()
		if processPath != "" {
			_ = chatsupervisor.SaveProcess(root, processPath, "exited", identity)
		}
		close(done)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			timer := time.NewTimer(time.Duration(c.Limits.LockWaitSeconds) * time.Second)
			defer timer.Stop()
			select {
			case <-done:
			case <-timer.C:
				_ = platform.KillGroup(child.Process.Pid)
				<-done
			}
		})
	}
	timer := time.NewTimer(time.Duration(c.Limits.LockWaitSeconds) * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(setupStartupPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return nil, ctx.Err()
		case <-timer.C:
			stop()
			return nil, errors.New("setup host did not become ready")
		case <-done:
			cancel()
			if exitErr == nil {
				exitErr = errors.New("setup host exited before readiness")
			}
			return nil, exitErr
		case <-ticker.C:
			if handled, err := check(); err != nil {
				stop()
				return nil, err
			} else if handled {
				return stop, nil
			}
		}
	}
}

// The maintenance loop restores only a setup host it owns. An existing worker
// keeps ownership of SQLite and is never killed or replaced by reception.
func maintainChatHost(ctx context.Context, cmd *cobra.Command, root string, c config.Config, reports *errorreport.Recorder) {
	var stop func()
	var mode bool
	var retryAfter time.Time
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	ticker := time.NewTicker(time.Duration(c.Limits.PollSeconds) * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		requested, modeErr := service.WorkerEnabled(root)
		if modeErr != nil {
			_, _ = reports.Record(ctx, "host_failed", errorreport.Correlation{})
		}
		if stop != nil && requested != mode && modeErr == nil {
			stop()
			stop = nil
			retryAfter = time.Time{}
		}
		_, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
		if err != nil || !handled {
			if stop != nil {
				stop()
				stop = nil
			}
			if time.Now().After(retryAfter) {
				stop, err = startOwnedSetupHost(ctx, cmd, root, c, chatsupervisor.ControllerPath)
				mode = requested
				if err != nil && ctx.Err() == nil {
					_, _ = reports.Record(ctx, "host_failed", errorreport.Correlation{})
					retryAfter = time.Now().Add(time.Duration(c.Limits.RetryDelaySeconds) * time.Second)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
