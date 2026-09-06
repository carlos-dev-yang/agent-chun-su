//go:build linux

package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const procStatStartIndex = 19 // Field 22, after pid and the parenthesized command.
func Identify(pid int) (ProcessIdentity, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ProcessIdentity{}, err
	}
	end := strings.LastIndexByte(string(b), ')')
	if end < 0 {
		return ProcessIdentity{}, errors.New("invalid process identity")
	}
	fields := strings.Fields(string(b[end+1:]))
	if len(fields) <= procStatStartIndex {
		return ProcessIdentity{}, errors.New("incomplete process identity")
	}
	return ProcessIdentity{PID: pid, Start: fields[procStatStartIndex]}, nil
}
