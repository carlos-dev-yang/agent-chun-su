//go:build darwin || linux

package webresearch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"chunsu/internal/files"
)

const webLockPoll = 50 * time.Millisecond

type webLock struct{ file *os.File }

// The controller owns controller.lock for its entire lifetime. Web settings
// and the search ledger use their own lock so chat remains independent of it.
func acquireWebLock(ctx context.Context, root string, wait time.Duration) (*webLock, error) {
	state := filepath.Join(root, "state")
	if err := files.RequirePrivateDir(state); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(state, "web.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, files.FileMode)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		file.Close()
		return nil, errors.New("web lock must be a private regular file")
	}
	if err := files.RequireOwner(info); err != nil {
		file.Close()
		return nil, err
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(webLockPoll)
	defer ticker.Stop()
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &webLock{file: file}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			file.Close()
			return nil, errors.New("another web operation is updating settings; retry later")
		case <-ticker.C:
		}
	}
}

func (lock *webLock) Close() error {
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	return lock.file.Close()
}
