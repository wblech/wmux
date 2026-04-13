package client

import (
	"os"
	"path/filepath"
)

// defaultBaseDir returns ~/.wmux as the default base directory.
func defaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "wmux")
	}
	return filepath.Join(home, ".wmux")
}

// resolveConfig fills in any empty paths using namespace-derived defaults.
// Explicit With* overrides are preserved.
func resolveConfig(cfg *config) {
	if cfg.baseDir == "" {
		cfg.baseDir = defaultBaseDir()
	}

	nsDir := filepath.Join(cfg.baseDir, cfg.namespace)

	if cfg.socket == "" {
		cfg.socket = filepath.Join(nsDir, "daemon.sock")
	}
	if cfg.tokenPath == "" {
		cfg.tokenPath = filepath.Join(nsDir, "daemon.token")
	}
	if cfg.dataDir == "" {
		cfg.dataDir = filepath.Join(nsDir, "sessions")
	}
}
