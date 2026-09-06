//go:build !darwin && !linux

package platform

import (
	"context"
	"errors"
	"os/exec"
)

type ProcessIdentity struct {
	PID   int    `json:"pid"`
	Start string `json:"start"`
}

func ProcessGroup(cmd *exec.Cmd) {}
func KillGroup(pid int) error    { return errors.New("process isolation unsupported on this platform") }
func GroupExists(pid int) bool   { return true }
func Identify(pid int) (ProcessIdentity, error) {
	return ProcessIdentity{}, errors.New("process identity unsupported on this platform")
}
func ReconcileProcess(ctx context.Context, record ProcessIdentity) error {
	return errors.New("process recovery unsupported on this platform")
}
