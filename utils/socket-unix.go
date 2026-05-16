//go:build linux || darwin

package utils

import "syscall"

func setSocketOptions(fd uintptr) {
	syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, DefaultBufferSize)
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, DefaultBufferSize)
}
