package client

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveConfig_Defaults(t *testing.T) {
	cfg := newConfig(WithBaseDir("/tmp/wmux-test"))
	resolveConfig(cfg)

	assert.Equal(t, filepath.Join("/tmp/wmux-test", "default", "daemon.sock"), cfg.socket)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "default", "daemon.token"), cfg.tokenPath)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "default", "sessions"), cfg.dataDir)
}

func TestResolveConfig_CustomNamespace(t *testing.T) {
	cfg := newConfig(WithBaseDir("/tmp/wmux-test"), WithNamespace("watchtower"))
	resolveConfig(cfg)

	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "daemon.sock"), cfg.socket)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "daemon.token"), cfg.tokenPath)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "sessions"), cfg.dataDir)
}

func TestResolveConfig_OverrideSocket(t *testing.T) {
	cfg := newConfig(
		WithBaseDir("/tmp/wmux-test"),
		WithNamespace("watchtower"),
		WithSocket("/custom/path.sock"),
	)
	resolveConfig(cfg)

	assert.Equal(t, "/custom/path.sock", cfg.socket)
	assert.Equal(t, filepath.Join("/tmp/wmux-test", "watchtower", "daemon.token"), cfg.tokenPath)
}

func TestResolveConfig_OverrideAll(t *testing.T) {
	cfg := newConfig(
		WithSocket("/a.sock"),
		WithTokenPath("/a.token"),
		WithDataDir("/a/data"),
	)
	resolveConfig(cfg)

	assert.Equal(t, "/a.sock", cfg.socket)
	assert.Equal(t, "/a.token", cfg.tokenPath)
	assert.Equal(t, "/a/data", cfg.dataDir)
}
