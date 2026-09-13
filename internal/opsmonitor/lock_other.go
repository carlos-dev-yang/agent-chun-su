//go:build !darwin && !linux

package opsmonitor

import (
	"context"
	"errors"
	"time"
)

type monitorLock struct{}

func acquire(context.Context, string, time.Duration) (*monitorLock, error) {
	return nil, errors.New("monitor locking is not supported on this platform")
}
func (*monitorLock) Close() error { return nil }
