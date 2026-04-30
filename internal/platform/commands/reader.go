package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Load reads and parses commands.json at the given path.
//
// Returns:
//   - zero-value File and nil error if the file does not exist (a session
//     that has never persisted any command).
//   - File and ErrUnsupportedVersion if the file's Version is greater than
//     CurrentVersion. The returned File is populated only with Version
//     and SessionID for diagnostic logging.
//   - File and a wrapped error for any other failure (corrupted JSON,
//     permission denied, etc).
func Load(path string) (File, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil //nolint:exhaustruct
		}
		return File{}, fmt.Errorf("commands.Load: read %q: %w", path, err) //nolint:exhaustruct
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("commands.Load: parse %q: %w", path, err) //nolint:exhaustruct
	}

	if f.Version > CurrentVersion {
		// Return a stub File so callers can log Version/SessionID
		// without a second read.
		return File{ //nolint:exhaustruct
			Version:   f.Version,
			SessionID: f.SessionID,
		}, ErrUnsupportedVersion
	}

	return f, nil
}
