//go:build windows

package utils

import (
	"syscall"
)

func setSocketOptions(fd uintptr) {
	syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1) // Disable Nagle's algorithm
	syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, DefaultBufferSize)
	syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, DefaultBufferSize)
}
