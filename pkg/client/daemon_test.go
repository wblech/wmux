package client

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDaemon(t *testing.T) {
	dir := shortTempDir(t)
	d, err := NewDaemon(
		WithBaseDir(dir),
		WithNamespace("test"),
	)
	require.NoError(t, err)
	require.NotNil(t, d)
}

func TestNewDaemon_ServeAndConnect(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := shortTempDir(t)
	d, err := NewDaemon(
		WithBaseDir(dir),
		WithNamespace("test"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Serve(ctx)
	}()

	// Wait for socket to be ready
	require.Eventually(t, func() bool {
		c, err := New(
			WithBaseDir(dir),
			WithNamespace("test"),
			WithAutoStart(false),
		)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}, 3*time.Second, 50*time.Millisecond)

	// Connect and use
	c, err := New(
		WithBaseDir(dir),
		WithNamespace("test"),
		WithAutoStart(false),
	)
	require.NoError(t, err)
	defer c.Close() //nolint:errcheck

	sessions, err := c.List()
	require.NoError(t, err)
	assert.Empty(t, sessions)

	cancel()
}

func TestServeDaemon_NotDaemonMode(t *testing.T) {
	handled := ServeDaemon([]string{"watchtower", "--some-flag"})
	assert.False(t, handled)
}

func TestIsDaemonMode(t *testing.T) {
	assert.True(t, isDaemonMode([]string{"watchtower", "__wmux_daemon__", "--namespace", "test"}))
	assert.False(t, isDaemonMode([]string{"watchtower", "--help"}))
	assert.False(t, isDaemonMode([]string{}))
}

func TestParseDaemonArgs(t *testing.T) {
	args := []string{"watchtower", "__wmux_daemon__", "--base-dir", "/tmp/test", "--namespace", "myapp", "--socket", "/tmp/s.sock", "--data-dir", "/tmp/data"}
	opts := parseDaemonArgs(args)

	cfg := newConfig(opts...)
	assert.Equal(t, "/tmp/test", cfg.baseDir)
	assert.Equal(t, "myapp", cfg.namespace)
	assert.Equal(t, "/tmp/s.sock", cfg.socket)
	assert.Equal(t, "/tmp/data", cfg.dataDir)
}
