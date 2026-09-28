//go:build darwin || linux

package conversationstate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
)

type lock struct{ file *os.File }

func acquire(ctx context.Context, root string, limits config.Limits) (*lock, error) {
	return acquireNamed(ctx, root, "state/conversation.lock", limits)
}

func acquireNamed(ctx context.Context, root, name string, limits config.Limits) (*lock, error) {
	if err := files.RequirePrivateDir(filepath.Join(root, filepath.Dir(name))); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, files.FileMode)
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
		return nil, errors.New("conversation lock must be a private regular file")
	}
	if err = files.RequireOwner(info); err != nil {
		file.Close()
		return nil, err
	}
	deadline := time.NewTimer(time.Duration(limits.LockWaitSeconds) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return &lock{file}, nil
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
			return nil, errors.New("conversation store is busy")
		case <-ticker.C:
		}
	}
}

func (l *lock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}
