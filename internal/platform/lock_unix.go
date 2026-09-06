//go:build darwin || linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"chunsu/internal/files"
)

const lockPoll = 50 * time.Millisecond

type Lock struct{ file *os.File }

func Acquire(ctx context.Context, root string, timeout time.Duration) (*Lock, error) {
	p := filepath.Join(root, "state", "controller.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, files.FileMode)
	if err != nil {
		return nil, err
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(lockPoll)
	defer tick.Stop()
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &Lock{file: f}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			f.Close()
			return nil, fmt.Errorf("another controller owns this data directory; use its management channel or wait")
		case <-tick.C:
		}
	}
}

func (l *Lock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}
