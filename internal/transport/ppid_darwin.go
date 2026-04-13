//go:build darwin

package transport

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

func platformParentPID(pid int32) (int32, error) {
	const kinfoStructSize = 648

	mib := [4]int32{1, 14, 1, pid} // CTL_KERN, KERN_PROC, KERN_PROC_PID

	buf := make([]byte, kinfoStructSize)
	size := uintptr(len(buf))

	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		4,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
		0,
	)

	if errno != 0 {
		return 0, fmt.Errorf("sysctl KERN_PROC_PID %d: %w", pid, errno)
	}

	if size < kinfoStructSize {
		return 0, fmt.Errorf("sysctl KERN_PROC_PID %d: short read (%d bytes)", pid, size)
	}

	// e_ppid offset in kinfo_proc on macOS arm64/amd64.
	ppid := int32(binary.LittleEndian.Uint32(buf[560:564]))

	return ppid, nil
}
