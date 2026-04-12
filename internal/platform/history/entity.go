// Package history provides file I/O for session history persistence.
package history

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Metadata holds the JSON-serializable session metadata written to meta.json.
type Metadata struct {
	// SessionID is the unique session identifier.
	SessionID string `json:"session_id"`
	// Shell is the path to the shell binary.
	Shell string `json:"shell"`
	// Cwd is the working directory at session creation.
	Cwd string `json:"cwd"`
	// Cols is the terminal width in columns at creation.
	Cols int `json:"cols"`
	// Rows is the terminal height in rows at creation.
	Rows int `json:"rows"`
	// StartedAt is when the session was created.
	StartedAt time.Time `json:"started_at"`
	// EndedAt is when the session exited (nil if still running).
	EndedAt *time.Time `json:"ended_at,omitempty"`
	// ExitCode is the shell process exit code (nil if still running).
	ExitCode *int `json:"exit_code,omitempty"`
}

// Sentinel errors for history operations.
var (
	// ErrInvalidSize is returned when a size string cannot be parsed.
	ErrInvalidSize = errors.New("invalid size string")
	// ErrSessionDirNotFound is returned when a session directory does not exist.
	ErrSessionDirNotFound = errors.New("session directory not found")
	// ErrWriterClosed is returned when writing to a closed ScrollbackWriter.
	ErrWriterClosed = errors.New("scrollback writer closed")
)

// size multipliers.
const (
	kilobyte = 1024
	megabyte = 1024 * 1024
	gigabyte = 1024 * 1024 * 1024
)

// ParseSize parses a human-readable size string into bytes.
// Accepted formats: "0", "1024" (bytes), "512KB", "1MB", "5GB".
// Case-insensitive. Returns ErrInvalidSize for invalid input.
func ParseSize(s string) (int64, error) {
	if s == "" {
		return 0, ErrInvalidSize
	}

	upper := strings.ToUpper(strings.TrimSpace(s))

	var multiplier int64 = 1
	numStr := upper

	switch {
	case strings.HasSuffix(upper, "GB"):
		multiplier = gigabyte
		numStr = strings.TrimSuffix(upper, "GB")
	case strings.HasSuffix(upper, "MB"):
		multiplier = megabyte
		numStr = strings.TrimSuffix(upper, "MB")
	case strings.HasSuffix(upper, "KB"):
		multiplier = kilobyte
		numStr = strings.TrimSuffix(upper, "KB")
	}

	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, ErrInvalidSize
	}

	if n < 0 {
		return 0, ErrInvalidSize
	}

	return n * multiplier, nil
}
