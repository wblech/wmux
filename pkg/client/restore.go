package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/platform/history"
)

// ErrColdRestoreNotAvailable is returned when no restorable history exists for a session.
var ErrColdRestoreNotAvailable = errors.New("client: cold restore not available")

// LoadSessionHistory reads the history files for a specific session from disk.
// Returns ErrColdRestoreNotAvailable if the session directory doesn't exist,
// has no metadata, or the session exited cleanly (endedAt is set).
func LoadSessionHistory(dataDir, sessionID string) (SessionHistory, error) {
	sessionDir := filepath.Join(dataDir, sessionID)

	meta, err := history.ReadMetadata(sessionDir)
	if err != nil {
		return SessionHistory{}, fmt.Errorf("%w: %s", ErrColdRestoreNotAvailable, err)
	}

	// Clean exit = not restorable.
	if meta.EndedAt != nil {
		return SessionHistory{}, fmt.Errorf("%w: session exited cleanly", ErrColdRestoreNotAvailable)
	}

	var scrollback []byte

	scrollbackPath := filepath.Join(sessionDir, "scrollback.bin")
	if data, readErr := os.ReadFile(scrollbackPath); readErr == nil {
		scrollback = data
	}

	return SessionHistory{
		Scrollback: scrollback,
		SessionID:  meta.SessionID,
		Shell:      meta.Shell,
		Cwd:        meta.Cwd,
		Cols:       meta.Cols,
		Rows:       meta.Rows,
	}, nil
}

// CleanSessionHistory removes the history files for a session after the
// integrator has consumed them. This is idempotent — returns nil if the
// directory doesn't exist.
func CleanSessionHistory(dataDir, sessionID string) error {
	sessionDir := filepath.Join(dataDir, sessionID)

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("client: clean history: %w", err)
	}

	return nil
}
