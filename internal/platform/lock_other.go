//go:build !darwin && !linux

package platform

import (
	"context"
	"errors"
	"time"
)

type Lock struct{}

func Acquire(context.Context, string, time.Duration) (*Lock, error) {
	return nil, errors.New("controller locking is not supported on this platform")
}
func (*Lock) Close() error { return nil }
