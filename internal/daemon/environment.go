package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ForwardEnv forwards an environment variable to a session directory.
// If the value is an existing file or socket path, creates a stable symlink.
// The symlink target is updated on each call (for re-attach scenarios).
func ForwardEnv(sessionDir, varName, value string) error {
	symlinkPath := filepath.Join(sessionDir, varName)

	if _, err := os.Stat(value); err != nil {
		return fmt.Errorf("environment: stat %q: %w", value, err)
	}

	_ = os.Remove(symlinkPath)

	if err := os.Symlink(value, symlinkPath); err != nil {
		return fmt.Errorf("environment: symlink %q -> %q: %w", symlinkPath, value, err)
	}

	return nil
}

// WriteEnvFile writes non-path environment variables to a sourceable env file.
// Format: KEY=VALUE per line, sorted for deterministic output.
func WriteEnvFile(sessionDir string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(env))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, env[k]))
	}

	path := filepath.Join(sessionDir, "env")
	content := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil { //nolint:gosec
		return fmt.Errorf("environment: write env file: %w", err)
	}

	return nil
}
