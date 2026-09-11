//go:build linux

package platform

import "golang.org/x/sys/unix"

const terminalGet = unix.TCGETS
const terminalSet = unix.TCSETS
