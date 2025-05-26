//go:build linux || darwin

package utils

import (
	"syscall"
)

func setSocketOptions(fd uintptr) {
	syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1) // Disable Nagle's algorithm
	// syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_WINDOW_CLAMP, 2*bufferSize)
	// syscall.SetsockoptString(int(fd), syscall.IPPROTO_TCP, syscall.TCP_CONGESTION, "cubic")
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, DefaultBufferSize)
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, DefaultBufferSize)
}
