//go:build darwin

package control

import "golang.org/x/sys/unix"

func peerUID(fd int) (int, error) {
	credential, err := unix.GetsockoptXucred(fd, unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return -1, err
	}
	return int(credential.Uid), nil
}
