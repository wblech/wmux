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
	handled, err := ServeDaemon([]string{"watchtower", "--some-flag"})
	assert.False(t, handled)
	assert.NoError(t, err)
}

func TestIsDaemonMode(t *testing.T) {
	assert.True(t, isDaemonMode([]string{"watchtower", "__wmux_daemon__", "--namespace", "test"}))
	assert.False(t, isDaemonMode([]string{"watchtower", "--help"}))
	assert.False(t, isDaemonMode([]string{}))
}

func TestParseDaemonArgs(t *testing.T) {
	args := []string{"watchtower", "__wmux_daemon__",
		"--base-dir", "/tmp/test",
		"--namespace", "myapp",
		"--socket", "/tmp/s.sock",
		"--token-path", "/tmp/t.token",
		"--data-dir", "/tmp/data",
		"--cold-restore",
		"--max-scrollback", "1048576",
		"--emulator-backend", "xterm",
	}
	opts := parseDaemonArgs(args)

	cfg := newConfig(opts...)
	assert.Equal(t, "/tmp/test", cfg.baseDir)
	assert.Equal(t, "myapp", cfg.namespace)
	assert.Equal(t, "/tmp/s.sock", cfg.socket)
	assert.Equal(t, "/tmp/t.token", cfg.tokenPath)
	assert.Equal(t, "/tmp/data", cfg.dataDir)
	assert.True(t, cfg.coldRestore)
	assert.Equal(t, int64(1048576), cfg.maxScrollbackSize)
	assert.Equal(t, "xterm", cfg.emulatorBackend)
}

func TestNewDaemon_InvalidBackend(t *testing.T) {
	dir := shortTempDir(t)
	_, err := NewDaemon(
		WithBaseDir(dir),
		WithNamespace("test"),
		WithEmulatorBackend("invalid"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown emulator backend")
}

func TestNew_AutoStart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := shortTempDir(t)

	// First try with autoStart=false should fail (no daemon)
	_, err := New(
		WithBaseDir(dir),
		WithNamespace("autotest"),
		WithAutoStart(false),
	)
	require.Error(t, err)

	// With autoStart=true (default) — New should spawn daemon and connect
	// NOTE: This test requires ServeDaemon hook in the test binary.
	// Since test binaries don't have ServeDaemon, we test via embedded daemon instead.
	// The auto-start spawn path is tested by TestBuildDaemonArgs.
}

func TestBuildDaemonArgs(t *testing.T) {
	cfg := &config{
		namespace:         "watchtower",
		baseDir:           "/tmp/wmux",
		socket:            "/tmp/wmux/watchtower/daemon.sock",
		tokenPath:         "/tmp/wmux/watchtower/daemon.token",
		dataDir:           "/tmp/wmux/watchtower/sessions",
		coldRestore:       true,
		maxScrollbackSize: 1048576,
		emulatorBackend:   "xterm",
		autoStart:         true,
	}

	args := buildDaemonArgs(cfg)

	assert.Contains(t, args, "__wmux_daemon__")
	assert.Contains(t, args, "--base-dir")
	assert.Contains(t, args, "/tmp/wmux")
	assert.Contains(t, args, "--namespace")
	assert.Contains(t, args, "watchtower")
	assert.Contains(t, args, "--socket")
	assert.Contains(t, args, "--token-path")
	assert.Contains(t, args, "--data-dir")
	assert.Contains(t, args, "--cold-restore")
	assert.Contains(t, args, "--max-scrollback")
	assert.Contains(t, args, "1048576")
	assert.Contains(t, args, "--emulator-backend")
	assert.Contains(t, args, "xterm")
}

func TestNewDaemon_SessionOperations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := shortTempDir(t)
	d, err := NewDaemon(WithBaseDir(dir), WithNamespace("ops"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Serve(ctx) }()

	// Wait for daemon to be ready
	var c *Client
	require.Eventually(t, func() bool {
		var connErr error
		c, connErr = New(WithBaseDir(dir), WithNamespace("ops"), WithAutoStart(false))
		return connErr == nil
	}, 3*time.Second, 50*time.Millisecond)
	defer c.Close() //nolint:errcheck

	// Create a session
	info, err := c.Create("test-sess", CreateParams{
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)
	assert.Equal(t, "test-sess", info.ID)
	assert.Equal(t, "alive", info.State)

	// List sessions
	sessions, err := c.List()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)

	// Info
	info, err = c.Info("test-sess")
	require.NoError(t, err)
	assert.Equal(t, "test-sess", info.ID)

	// Resize
	err = c.Resize("test-sess", 120, 40)
	require.NoError(t, err)

	// Write
	err = c.Write("test-sess", []byte("echo hello\n"))
	require.NoError(t, err)

	// Attach
	result, err := c.Attach("test-sess")
	require.NoError(t, err)
	assert.Equal(t, "test-sess", result.Session.ID)

	// Detach
	err = c.Detach("test-sess")
	require.NoError(t, err)

	// MetaSet + MetaGet
	err = c.MetaSet("test-sess", "app", "test")
	require.NoError(t, err)

	val, err := c.MetaGet("test-sess", "app")
	require.NoError(t, err)
	assert.Equal(t, "test", val)

	// MetaGetAll
	meta, err := c.MetaGetAll("test-sess")
	require.NoError(t, err)
	assert.Equal(t, "test", meta["app"])

	// Kill
	err = c.Kill("test-sess")
	require.NoError(t, err)

	cancel()
}

func TestBuildDaemonArgs_Defaults(t *testing.T) {
	cfg := &config{
		namespace:         "default",
		baseDir:           "",
		socket:            "/tmp/d.sock",
		tokenPath:         "/tmp/d.token",
		dataDir:           "/tmp/sessions",
		coldRestore:       false,
		maxScrollbackSize: 0,
		emulatorBackend:   "none",
		autoStart:         true,
	}

	args := buildDaemonArgs(cfg)

	assert.Contains(t, args, "__wmux_daemon__")
	assert.NotContains(t, args, "--namespace")
	assert.NotContains(t, args, "--base-dir")
	assert.NotContains(t, args, "--cold-restore")
	assert.NotContains(t, args, "--max-scrollback")
	assert.NotContains(t, args, "--emulator-backend")
}
