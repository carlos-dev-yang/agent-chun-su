//go:build linux

package control

import "golang.org/x/sys/unix"

func peerUID(fd int) (int, error) {
	credential, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return -1, err
	}
	return int(credential.Uid), nil
}
