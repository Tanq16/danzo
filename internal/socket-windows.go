//go:build windows

package internal

import (
	"syscall"
)

func setSocketOptions(fd uintptr) {
	syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 1024*1024)
	syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, 1024*1024)
}
