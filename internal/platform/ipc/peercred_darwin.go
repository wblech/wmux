//go:build darwin

package ipc

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// ExtractPeerCredentials retrieves the UID and PID of the peer process
// connected via a Unix domain socket on macOS.
func ExtractPeerCredentials(c net.Conn) (PeerCredentials, error) {
	uc, err := toUnixConn(c)
	if err != nil {
		return PeerCredentials{}, err
	}

	raw, err := uc.SyscallConn()
	if err != nil {
		return PeerCredentials{}, fmt.Errorf("ipc: syscall conn: %w", err)
	}

	var creds PeerCredentials
	var credErr error

	controlErr := raw.Control(func(fd uintptr) {
		xucred, xErr := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if xErr != nil {
			credErr = fmt.Errorf("ipc: getsockopt LOCAL_PEERCRED: %w", xErr)
			return
		}

		pid, pErr := unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if pErr != nil {
			credErr = fmt.Errorf("ipc: getsockopt LOCAL_PEERPID: %w", pErr)
			return
		}

		creds = PeerCredentials{
			UID: xucred.Uid,
			PID: int32(pid),
		}
	})

	if controlErr != nil {
		return PeerCredentials{}, fmt.Errorf("ipc: control: %w", controlErr)
	}

	if credErr != nil {
		return PeerCredentials{}, credErr
	}

	return creds, nil
}
