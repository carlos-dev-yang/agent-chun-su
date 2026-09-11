//go:build !darwin && !linux

package control

import (
	"errors"
	"net"
)

func verifyPeer(net.Conn) error {
	return errors.New("authenticated local management is unavailable on this platform")
}
