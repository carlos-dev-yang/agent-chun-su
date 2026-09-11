//go:build !darwin && !linux

package files

import (
	"errors"
	"os"
)

func OwnerUID(os.FileInfo) (int, error) {
	return -1, errors.New("OS ownership checks are unavailable on this platform")
}
func RequireOwner(os.FileInfo) error {
	return errors.New("OS ownership checks are unavailable on this platform")
}
