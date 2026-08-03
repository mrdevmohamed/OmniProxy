//go:build linux || android

package engine

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

const ifReqSize = unix.IFNAMSIZ + 64

// tunName returns the interface name of an established TUN fd.
func tunName(fd int) (string, error) {
	var ifr [ifReqSize]byte
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.TUNGETIFF),
		uintptr(unsafe.Pointer(&ifr[0])),
	)
	if errno != 0 {
		return "", fmt.Errorf("engine: get tun name: %w", errno)
	}
	return unix.ByteSliceToString(ifr[:]), nil
}
