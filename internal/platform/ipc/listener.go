// Package ipc provides Unix domain socket IPC primitives for wmux.
package ipc

import (
	"fmt"
	"net"
	"os"
)

// Listener wraps a net.UnixListener with automatic socket cleanup.
type Listener struct {
	inner *net.UnixListener
	path  string
}

// Listen creates a Unix domain socket listener at the given path.
// If a stale socket file exists at path, it is removed before binding.
func Listen(path string) (*Listener, error) {
	if _, err := os.Stat(path); err == nil {
		if removeErr := os.Remove(path); removeErr != nil {
			return nil, fmt.Errorf("ipc: remove stale socket %q: %w", path, removeErr)
		}
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, fmt.Errorf("ipc: resolve addr %q: %w", path, err)
	}

	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen %q: %w", path, err)
	}

	return &Listener{
		inner: ln,
		path:  path,
	}, nil
}

// Accept waits for and returns the next connection.
func (l *Listener) Accept() (net.Conn, error) {
	conn, err := l.inner.AcceptUnix()
	if err != nil {
		return nil, fmt.Errorf("ipc: accept: %w", err)
	}

	return conn, nil
}

// Close closes the listener and removes the socket file.
func (l *Listener) Close() error {
	if err := l.inner.Close(); err != nil {
		return fmt.Errorf("ipc: close listener: %w", err)
	}

	_ = os.Remove(l.path)

	return nil
}

// Addr returns the socket file path.
func (l *Listener) Addr() string {
	return l.path
}
