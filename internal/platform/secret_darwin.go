//go:build darwin

package platform

import "golang.org/x/sys/unix"

const terminalGet = unix.TIOCGETA
const terminalSet = unix.TIOCSETA
