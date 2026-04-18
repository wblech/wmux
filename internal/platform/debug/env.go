package debug

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// EnvConfig holds debug configuration parsed from environment variables.
type EnvConfig struct {
	Enabled   bool
	Path      string
	Level     Level
	MaxSizeMB int
	MaxFiles  int
}

// ReadEnv reads debug configuration from environment variables.
// WMUX_DEBUG=1 or WMUX_DEBUG_PATH=... enables debug tracing.
// Returns a zero EnvConfig (Enabled=false) when no env vars are set.
func ReadEnv() EnvConfig {
	cfg := EnvConfig{
		MaxSizeMB: 50,
		MaxFiles:  7,
	}

	path := os.Getenv("WMUX_DEBUG_PATH")
	debug := os.Getenv("WMUX_DEBUG")

	if path == "" && debug != "1" {
		return cfg
	}

	cfg.Enabled = true

	if path != "" {
		cfg.Path = path
	} else {
		cfg.Path = defaultLogPath()
	}

	cfg.Level = LevelChunk // default when enabled

	if s := os.Getenv("WMUX_DEBUG_LEVEL"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			cfg.Level = ClampLevel(n)
		}
	}

	if s := os.Getenv("WMUX_DEBUG_MAX_SIZE_MB"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.MaxSizeMB = n
		}
	}

	if s := os.Getenv("WMUX_DEBUG_MAX_FILES"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.MaxFiles = n
		}
	}

	return cfg
}

// ClampLevel restricts a raw int to the valid Level range [0, 3].
func ClampLevel(n int) Level {
	if n < 0 {
		return LevelOff
	}
	if n > int(LevelFull) {
		return LevelFull
	}
	return Level(n)
}

// defaultLogPath returns the platform-appropriate default debug log path.
func defaultLogPath() string {
	home, _ := os.UserHomeDir()

	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "wmux", "wmux-debug.log")
	}

	// Linux / other: XDG_STATE_HOME or fallback.
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		stateDir = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(stateDir, "wmux", "wmux-debug.log")
}
