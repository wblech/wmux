package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "wmux.toml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)

	// daemon
	assert.Equal(t, "~/.wmux/daemon.sock", cfg.Daemon.Socket)
	assert.Equal(t, 0, cfg.Daemon.MaxSessions)
	assert.Equal(t, "0", cfg.Daemon.IdleTTL)
	assert.Equal(t, "0", cfg.Daemon.RSSWarning)
	assert.False(t, cfg.Daemon.RemainOnExit)
	assert.Equal(t, 3, cfg.Daemon.MaxConcurrentSpawns)
	assert.Equal(t, "same-user", cfg.Daemon.AutomationMode)

	// emulator
	assert.Equal(t, "none", cfg.Emulator.Backend)

	// history
	assert.Equal(t, "0", cfg.History.MaxPerSession)
	assert.Equal(t, "0", cfg.History.MaxTotal)
	assert.False(t, cfg.History.Recording)

	// backpressure
	assert.Equal(t, "1MB", cfg.Backpressure.HighWatermark)
	assert.Equal(t, "256KB", cfg.Backpressure.LowWatermark)
	assert.Equal(t, "16ms", cfg.Backpressure.BatchInterval)

	// heartbeat
	assert.Equal(t, "10s", cfg.Heartbeat.Interval)
	assert.Equal(t, 3, cfg.Heartbeat.MaxMissed)

	// reaper
	assert.Equal(t, "5m", cfg.Reaper.CheckInterval)

	// environment
	assert.Equal(t, []string{"SSH_AUTH_SOCK", "SSH_CONNECTION", "DISPLAY"}, cfg.Environment.Update)

	// shell
	assert.Equal(t, "", cfg.Shell.Default)
	assert.False(t, cfg.Shell.UseWrapper)

	// watchdog
	assert.Equal(t, "30s", cfg.Watchdog.Timeout)

	// resize
	assert.Equal(t, "leader", cfg.Resize.Strategy)
}

func TestLoad_CustomValues(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "wmux.toml")
	content := `
[daemon]
socket = "/tmp/test.sock"
max_sessions = 10
idle_ttl = "1h"
rss_warning = "512MB"
remain_on_exit = true
max_concurrent_spawns = 5
automation_mode = "any-user"

[emulator]
backend = "tmux"

[history]
max_per_session = "1000"
max_total = "5000"
recording = true

[backpressure]
high_watermark = "2MB"
low_watermark = "512KB"
batch_interval = "32ms"

[heartbeat]
interval = "30s"
max_missed = 5

[reaper]
check_interval = "10m"

[environment]
update = ["HOME", "USER"]

[shell]
default = "/bin/zsh"
use_wrapper = true

[watchdog]
timeout = "60s"

[resize]
strategy = "follower"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/test.sock", cfg.Daemon.Socket)
	assert.Equal(t, 10, cfg.Daemon.MaxSessions)
	assert.Equal(t, "1h", cfg.Daemon.IdleTTL)
	assert.Equal(t, "512MB", cfg.Daemon.RSSWarning)
	assert.True(t, cfg.Daemon.RemainOnExit)
	assert.Equal(t, 5, cfg.Daemon.MaxConcurrentSpawns)
	assert.Equal(t, "any-user", cfg.Daemon.AutomationMode)

	assert.Equal(t, "tmux", cfg.Emulator.Backend)

	assert.Equal(t, "1000", cfg.History.MaxPerSession)
	assert.Equal(t, "5000", cfg.History.MaxTotal)
	assert.True(t, cfg.History.Recording)

	assert.Equal(t, "2MB", cfg.Backpressure.HighWatermark)
	assert.Equal(t, "512KB", cfg.Backpressure.LowWatermark)
	assert.Equal(t, "32ms", cfg.Backpressure.BatchInterval)

	assert.Equal(t, "30s", cfg.Heartbeat.Interval)
	assert.Equal(t, 5, cfg.Heartbeat.MaxMissed)

	assert.Equal(t, "10m", cfg.Reaper.CheckInterval)

	assert.Equal(t, []string{"HOME", "USER"}, cfg.Environment.Update)

	assert.Equal(t, "/bin/zsh", cfg.Shell.Default)
	assert.True(t, cfg.Shell.UseWrapper)

	assert.Equal(t, "60s", cfg.Watchdog.Timeout)

	assert.Equal(t, "follower", cfg.Resize.Strategy)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/wmux.toml")
	require.Error(t, err)
}

func TestWatch_ReloadsOnChange(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "wmux.toml")

	initial := `
[daemon]
socket = "/tmp/initial.sock"
`
	require.NoError(t, os.WriteFile(path, []byte(initial), 0o600))

	changed := make(chan *Config, 1)
	stop, err := Watch(path, func(cfg *Config) {
		changed <- cfg
	})
	require.NoError(t, err)
	defer stop()

	// Give the watcher time to register the initial state
	time.Sleep(600 * time.Millisecond)

	updated := `
[daemon]
socket = "/tmp/updated.sock"
`
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))

	select {
	case cfg := <-changed:
		assert.Equal(t, "/tmp/updated.sock", cfg.Daemon.Socket)
	case <-time.After(3 * time.Second):
		t.Fatal("onChange was not called within 3 seconds")
	}
}
