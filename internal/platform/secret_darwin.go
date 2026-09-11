//go:build darwin

package platform

import (
	"bufio"
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

const terminalDevice = "/dev/tty"

// ReadSecret disables terminal echo and restores it on success or cancellation.
// It never returns terminal diagnostics containing the input.
func ReadSecret(ctx context.Context, limit int) (string, error) {
	tty, e := os.OpenFile(terminalDevice, os.O_RDWR, 0)
	if e != nil {
		return "", errors.New("비밀 입력은 로컬 터미널에서 실행해 주세요")
	}
	defer tty.Close()
	fd := int(tty.Fd())
	original, e := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if e != nil {
		return "", errors.New("cannot inspect terminal echo")
	}
	hidden := *original
	hidden.Lflag &^= unix.ECHO
	if e = unix.IoctlSetTermios(fd, unix.TIOCSETA, &hidden); e != nil {
		return "", errors.New("cannot disable terminal echo")
	}
	defer unix.IoctlSetTermios(fd, unix.TIOCSETA, original)
	type result struct {
		value string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		s := bufio.NewScanner(tty)
		s.Buffer(nil, limit+1)
		if s.Scan() {
			done <- result{s.Text(), nil}
		} else {
			done <- result{"", errors.New("secret input ended or exceeded the limit")}
		}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		if len(r.value) > limit {
			return "", errors.New("secret input exceeds the limit")
		}
		return r.value, r.err
	}
}
