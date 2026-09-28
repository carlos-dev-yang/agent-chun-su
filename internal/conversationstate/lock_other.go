//go:build !darwin && !linux

package conversationstate

import (
	"context"
	"errors"

	"chunsu/internal/config"
)

type lock struct{}

func acquire(context.Context, string, config.Limits) (*lock, error) {
	return nil, errors.New("conversation locking is unavailable on this platform")
}
func acquireNamed(context.Context, string, string, config.Limits) (*lock, error) {
	return nil, errors.New("conversation locking is unavailable on this platform")
}
func (*lock) Close() error { return nil }
