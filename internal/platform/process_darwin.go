//go:build darwin

package platform

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
)

func Identify(pid int) (ProcessIdentity, error) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ProcessIdentity{}, err
	}
	if p.Proc.P_pid != int32(pid) {
		return ProcessIdentity{}, errors.New("process is no longer present")
	}
	return ProcessIdentity{PID: pid, Start: fmt.Sprintf("%d:%d", p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec)}, nil
}
