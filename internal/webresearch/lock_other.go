//go:build !darwin && !linux

package webresearch

import (
	"context"
	"errors"
	"time"
)

type webLock struct{}

func acquireWebLock(context.Context, string, time.Duration) (*webLock, error) {
	return nil, errors.New("web locking is not supported on this platform")
}

func (*webLock) Close() error { return nil }
