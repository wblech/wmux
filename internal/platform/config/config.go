package config

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// DaemonConfig holds daemon process settings.
type DaemonConfig struct {
	Socket              string `koanf:"socket"`
	MaxSessions         int    `koanf:"max_sessions"`
	IdleTTL             string `koanf:"idle_ttl"`
	RSSWarning          string `koanf:"rss_warning"`
	RemainOnExit        bool   `koanf:"remain_on_exit"`
	MaxConcurrentSpawns int    `koanf:"max_concurrent_spawns"`
	AutomationMode      string `koanf:"automation_mode"`
}

// EmulatorConfig holds terminal emulator settings.
type EmulatorConfig struct {
	Backend string `koanf:"backend"`
}

// HistoryConfig holds session history settings.
type HistoryConfig struct {
	MaxPerSession string `koanf:"max_per_session"`
	MaxTotal      string `koanf:"max_total"`
	Recording     bool   `koanf:"recording"`
}

// BackpressureConfig holds backpressure flow control settings.
type BackpressureConfig struct {
	HighWatermark string `koanf:"high_watermark"`
	LowWatermark  string `koanf:"low_watermark"`
	BatchInterval string `koanf:"batch_interval"`
}

// HeartbeatConfig holds keepalive settings.
type HeartbeatConfig struct {
	Interval  string `koanf:"interval"`
	MaxMissed int    `koanf:"max_missed"`
}

// ReaperConfig holds dead session cleanup settings.
type ReaperConfig struct {
	CheckInterval string `koanf:"check_interval"`
}

// EnvironmentConfig holds environment variable propagation settings.
type EnvironmentConfig struct {
	Update []string `koanf:"update"`
}

// ShellConfig holds shell invocation settings.
type ShellConfig struct {
	Default    string `koanf:"default"`
	UseWrapper bool   `koanf:"use_wrapper"`
}

// WatchdogConfig holds unresponsive session detection settings.
type WatchdogConfig struct {
	Timeout string `koanf:"timeout"`
}

// ResizeConfig holds terminal resize propagation settings.
type ResizeConfig struct {
	Strategy string `koanf:"strategy"`
}

// Config is the top-level configuration structure for wmux.
type Config struct {
	Daemon       DaemonConfig       `koanf:"daemon"`
	Emulator     EmulatorConfig     `koanf:"emulator"`
	History      HistoryConfig      `koanf:"history"`
	Backpressure BackpressureConfig `koanf:"backpressure"`
	Heartbeat    HeartbeatConfig    `koanf:"heartbeat"`
	Reaper       ReaperConfig       `koanf:"reaper"`
	Environment  EnvironmentConfig  `koanf:"environment"`
	Shell        ShellConfig        `koanf:"shell"`
	Watchdog     WatchdogConfig     `koanf:"watchdog"`
	Resize       ResizeConfig       `koanf:"resize"`
}

// defaults returns a Config populated with all PRD-specified default values.
func defaults() *Config {
	return &Config{
		Daemon: DaemonConfig{
			Socket:              "~/.wmux/daemon.sock",
			MaxSessions:         0,
			IdleTTL:             "0",
			RSSWarning:          "0",
			RemainOnExit:        false,
			MaxConcurrentSpawns: 3,
			AutomationMode:      "same-user",
		},
		Emulator: EmulatorConfig{
			Backend: "none",
		},
		History: HistoryConfig{
			MaxPerSession: "0",
			MaxTotal:      "0",
			Recording:     false,
		},
		Backpressure: BackpressureConfig{
			HighWatermark: "1MB",
			LowWatermark:  "256KB",
			BatchInterval: "16ms",
		},
		Heartbeat: HeartbeatConfig{
			Interval:  "10s",
			MaxMissed: 3,
		},
		Reaper: ReaperConfig{
			CheckInterval: "5m",
		},
		Environment: EnvironmentConfig{
			Update: []string{"SSH_AUTH_SOCK", "SSH_CONNECTION", "DISPLAY"},
		},
		Shell: ShellConfig{
			Default:    "",
			UseWrapper: false,
		},
		Watchdog: WatchdogConfig{
			Timeout: "30s",
		},
		Resize: ResizeConfig{
			Strategy: "leader",
		},
	}
}

// Load reads a TOML configuration file from path and returns a Config with
// all missing fields populated with their PRD defaults.
func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
		return nil, fmt.Errorf("config: load %q: %w", path, err)
	}

	cfg := defaults()
	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	return cfg, nil
}

// Watch polls the config file at path every 500ms and calls onChange with the
// newly-parsed Config whenever the file content changes. It returns a stop
// function that cancels the polling goroutine.
func Watch(path string, onChange func(*Config)) (stop func(), err error) {
	// Verify the file is readable before starting the watcher.
	if _, err := Load(path); err != nil {
		return nil, err
	}

	hash, err := fileHash(path)
	if err != nil {
		return nil, fmt.Errorf("config: watch hash: %w", err)
	}

	done := make(chan struct{})
	stop = func() {
		close(done)
	}

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		current := hash
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				h, err := fileHash(path)
				if err != nil || h == current {
					continue
				}
				current = h
				cfg, err := Load(path)
				if err != nil {
					continue
				}
				onChange(cfg)
			}
		}
	}()

	return stop, nil
}

// fileHash returns a hex SHA-256 digest of the file at path.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
