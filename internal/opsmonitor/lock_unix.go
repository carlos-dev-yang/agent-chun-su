//go:build darwin || linux

package opsmonitor

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

type monitorLock struct{ file *os.File }

func acquire(ctx context.Context, root string, timeout time.Duration) (*monitorLock, error) {
	if timeout <= 0 {
		return nil, errors.New("monitor lock timeout must be positive")
	}
	if err := files.PrivateDir(filepath.Join(root, Directory)); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, Directory, "monitor.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, files.FileMode)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("monitor lock must be a private regular file")
	}
	if err = files.RequireOwner(info); err != nil {
		_ = file.Close()
		return nil, err
	}
	deadline, tick := time.NewTimer(timeout), time.NewTicker(lockPoll)
	defer deadline.Stop()
	defer tick.Stop()
	for {
		if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return &monitorLock{file: file}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			_ = file.Close()
			return nil, fmt.Errorf("another monitor owns this data directory")
		case <-tick.C:
		}
	}
}

func (l *monitorLock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}
