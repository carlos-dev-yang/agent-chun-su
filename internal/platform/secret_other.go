//go:build !darwin

package platform

import (
	"context"
	"errors"
)

func ReadSecret(context.Context, int) (string, error) {
	return "", errors.New("interactive secret setup currently requires macOS")
}
