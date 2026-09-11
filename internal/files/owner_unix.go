//go:build darwin || linux

package files

import (
	"errors"
	"os"
	"syscall"
)

func OwnerUID(info os.FileInfo) (int, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, errors.New("cannot verify filesystem owner")
	}
	return int(stat.Uid), nil
}

func RequireOwner(info os.FileInfo) error {
	uid, err := OwnerUID(info)
	if err != nil {
		return err
	}
	if uid != os.Geteuid() {
		return errors.New("managed data belongs to another OS user")
	}
	return nil
}
