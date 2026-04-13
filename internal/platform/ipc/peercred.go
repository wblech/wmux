package ipc

import (
	"errors"
	"net"
)

// PeerCredentials holds the credentials of the process on the other end of a
// Unix domain socket connection.
type PeerCredentials struct {
	// UID is the effective user ID of the peer process.
	UID uint32
	// PID is the process ID of the peer process.
	PID int32
}

// ErrNotUnixConn is returned when the connection is not a Unix domain socket.
var ErrNotUnixConn = errors.New("ipc: connection is not a Unix domain socket")

// toUnixConn extracts the *net.UnixConn from a net.Conn.
func toUnixConn(c net.Conn) (*net.UnixConn, error) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return nil, ErrNotUnixConn
	}

	return uc, nil
}
