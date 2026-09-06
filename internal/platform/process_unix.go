//go:build darwin || linux

package platform

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

type ProcessIdentity struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
}

func ProcessGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func KillGroup(pid int) error {
	if pid <= 1 {
		return errors.New("invalid process group")
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func GroupExists(pid int) bool {
	if pid <= 1 {
		return false
	}
	return !errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH)
}

func ReconcileProcess(ctx context.Context, record ProcessIdentity) error {
	if !GroupExists(record.PID) {
		return nil
	}
	actual, err := Identify(record.PID)
	if err != nil || actual.Start != record.Start || actual.Start == "" {
		return errors.New("an orphan process group cannot be identified safely; inspect it before retrying")
	}
	if err = KillGroup(record.PID); err != nil {
		return err
	}
	tick := time.NewTicker(lockPoll)
	defer tick.Stop()
	for GroupExists(record.PID) {
		select {
		case <-ctx.Done():
			return errors.New("process cleanup could not be confirmed")
		case <-tick.C:
		}
	}
	return nil
}
