//go:build darwin

package ipc

import (
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeClosedUnixConn returns a *net.UnixConn backed by a closed file descriptor.
// Calling ExtractPeerCredentials on it exercises the credErr/controlErr paths.
func makeClosedUnixConn(t *testing.T) *net.UnixConn {
	t.Helper()

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	require.NoError(t, err)

	// Wrap the second fd in an os.File and immediately close it so that
	// the duplicate fd inside the resulting net.Conn is invalid for syscalls.
	f := os.NewFile(uintptr(fds[1]), "sock1")
	require.NotNil(t, f)

	conn, err := net.FileConn(f)
	require.NoError(t, err)

	// Close both the os.File wrapper and the original fd to make the
	// underlying socket invalid when SyscallConn.Control fires.
	require.NoError(t, f.Close())
	require.NoError(t, syscall.Close(fds[0]))

	uc, ok := conn.(*net.UnixConn)
	require.True(t, ok)

	return uc
}

func TestExtractPeerCredentials_CredErr_InvalidFD(t *testing.T) {
	uc := makeClosedUnixConn(t)
	t.Cleanup(func() { _ = uc.Close() })

	// The getsockopt calls will fail because the socket has no peer,
	// exercising the credErr return path.
	_, err := ExtractPeerCredentials(uc)
	assert.Error(t, err)
}

func TestExtractPeerCredentials_ControlErr(t *testing.T) {
	uc := makeClosedUnixConn(t)

	// Fully close the conn so that raw.Control() returns an error.
	require.NoError(t, uc.Close())

	// raw.Control on a closed conn returns "use of closed network connection",
	// exercising the controlErr != nil return path.
	_, err := ExtractPeerCredentials(uc)
	assert.Error(t, err)
}
