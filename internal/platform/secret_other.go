//go:build !darwin && !linux

package platform

import (
	"context"
	"errors"
)

func ReadSecret(context.Context, int) (string, error) {
	return "", errors.New("interactive secret setup requires a supported local terminal")
}
