package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/control"
	"chunsu/internal/platform"
	"github.com/spf13/cobra"
)

// startOwnedSetupHost is only for an explicit local setup conversation. Chat
// transports and ordinary local chat never own or replace a controller.
func startOwnedSetupHost(ctx context.Context, cmd *cobra.Command, root string, c config.Config) (func(), error) {
	check := func() (bool, error) {
		_, handled, err := control.Call(ctx, root, time.Duration(c.Limits.LockWaitSeconds)*time.Second, c.Limits.MaxArtifactBytes, control.Request{Operation: "status"})
		return handled, err
	}
	if handled, err := check(); handled || err != nil {
		return func() {}, err
	}
	binary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	childCtx, cancel := context.WithCancel(ctx)
	child := exec.CommandContext(childCtx, binary, "--home", root, "setup", "serve")
	platform.ProcessGroup(child)
	child.Stderr = cmd.ErrOrStderr()
	child.Cancel = func() error { return child.Process.Signal(os.Interrupt) }
	child.WaitDelay = time.Duration(c.Limits.LockWaitSeconds) * time.Second
	if err = child.Start(); err != nil {
		cancel()
		return nil, err
	}
	done := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = child.Wait()
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
