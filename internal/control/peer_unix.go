//go:build darwin || linux

package control

import (
	"errors"
	"net"
	"os"
)

func verifyPeer(connection net.Conn) error {
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return errors.New("management requires a local Unix connection")
	}
	raw, err := unixConnection.SyscallConn()
	if err != nil {
		return err
	}
	var peer int
	var checkErr error
	if err = raw.Control(func(fd uintptr) { peer, checkErr = peerUID(int(fd)) }); err != nil {
		return err
	}
	if checkErr != nil {
		return checkErr
	}
	if peer != os.Geteuid() {
		return errors.New("management peer belongs to another OS user")
	}
	return nil
}
