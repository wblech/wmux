//go:build linux

package ipc

import (
	"fmt"
	"net"
	"syscall"
)

// ExtractPeerCredentials retrieves the UID and PID of the peer process
// connected via a Unix domain socket on Linux using SO_PEERCRED.
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
		ucred, gErr := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if gErr != nil {
			credErr = fmt.Errorf("ipc: getsockopt SO_PEERCRED: %w", gErr)
			return
		}

		creds = PeerCredentials{
			UID: ucred.Uid,
			PID: ucred.Pid,
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
