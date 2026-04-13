package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaults(t *testing.T) {
	cfg := newConfig()
	assert.Equal(t, "default", cfg.namespace)
	assert.Empty(t, cfg.socket)
	assert.Empty(t, cfg.tokenPath)
	assert.Empty(t, cfg.dataDir)
	assert.Empty(t, cfg.baseDir)
	assert.True(t, cfg.autoStart)
	assert.False(t, cfg.coldRestore)
	assert.Equal(t, int64(0), cfg.maxScrollbackSize)
	assert.Equal(t, "none", cfg.emulatorBackend)
}

func TestWithNamespace(t *testing.T) {
	cfg := newConfig(WithNamespace("watchtower"))
	assert.Equal(t, "watchtower", cfg.namespace)
}

func TestWithSocket(t *testing.T) {
	cfg := newConfig(WithSocket("/custom/path.sock"))
	assert.Equal(t, "/custom/path.sock", cfg.socket)
}

func TestWithTokenPath(t *testing.T) {
	cfg := newConfig(WithTokenPath("/custom/token"))
	assert.Equal(t, "/custom/token", cfg.tokenPath)
}

func TestWithDataDir(t *testing.T) {
	cfg := newConfig(WithDataDir("/custom/data"))
	assert.Equal(t, "/custom/data", cfg.dataDir)
}

func TestWithBaseDir(t *testing.T) {
	cfg := newConfig(WithBaseDir("/custom/base"))
	assert.Equal(t, "/custom/base", cfg.baseDir)
}

func TestWithAutoStart(t *testing.T) {
	cfg := newConfig(WithAutoStart(false))
	assert.False(t, cfg.autoStart)
}

func TestWithColdRestore(t *testing.T) {
	cfg := newConfig(WithColdRestore(true))
	assert.True(t, cfg.coldRestore)
}

func TestWithMaxScrollbackSize(t *testing.T) {
	cfg := newConfig(WithMaxScrollbackSize(10 * 1024 * 1024))
	assert.Equal(t, int64(10*1024*1024), cfg.maxScrollbackSize)
}

func TestWithEmulatorBackend(t *testing.T) {
	cfg := newConfig(WithEmulatorBackend("xterm"))
	assert.Equal(t, "xterm", cfg.emulatorBackend)
}

func TestMultipleOptions(t *testing.T) {
	cfg := newConfig(
		WithNamespace("watchtower"),
		WithColdRestore(true),
		WithEmulatorBackend("xterm"),
		WithMaxScrollbackSize(1024),
	)
	assert.Equal(t, "watchtower", cfg.namespace)
	assert.True(t, cfg.coldRestore)
	assert.Equal(t, "xterm", cfg.emulatorBackend)
	assert.Equal(t, int64(1024), cfg.maxScrollbackSize)
}
