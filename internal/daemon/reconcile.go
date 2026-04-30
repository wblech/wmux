package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/commands"
	"github.com/wblech/wmux/internal/platform/history"
)

// ReconcileOrphans scans the history data directory for sessions with missing
// endedAt (indicating unclean daemon shutdown) and updates their metadata to
// mark them as exited with exit code -1.
// Returns the number of orphaned sessions reconciled.
func ReconcileOrphans(dataDir string) (int, error) {
	dirs, err := history.ListSessionDirs(dataDir)
	if err != nil {
		return 0, fmt.Errorf("reconcile: list session dirs: %w", err)
	}

	reconciled := 0

	for _, dir := range dirs {
		if dir.EndedAt != nil {
			continue // Already completed.
		}

		now := time.Now()
		exitCode := -1

		if err := history.UpdateMetadataExit(dir.Path, now, exitCode); err != nil {
			continue // Best effort — skip broken metadata.
		}

		reconciled++
	}

	return reconciled, nil
}

// RestoreCommandsForSession loads the per-session commands.json (if
// present) and rehydrates the cmdlifecycle Service for the session.
//
// Returns nil if the file is missing, present-but-future-version, or
// loaded successfully. Returns an error only for unexpected I/O errors
// or when the cmdlifecycle Service rejects the Restore.
func RestoreCommandsForSession(dataDir, sessID string, cmdSvc *cmdlifecycle.Service) error {
	if cmdSvc == nil {
		return nil
	}
	path := filepath.Join(dataDir, sessID, "commands.json")
	file, err := commands.Load(path)
	if err != nil {
		if errors.Is(err, commands.ErrUnsupportedVersion) {
			fmt.Fprintf(os.Stderr, "wmux: skip session %q: commands.json version %d unsupported\n", sessID, file.Version)
			return nil
		}
		return fmt.Errorf("RestoreCommandsForSession: %w", err)
	}
	// Empty file (file did not exist): nothing to restore.
	if file.Version == 0 && len(file.History) == 0 && file.InFlight == nil {
		return nil
	}

	state := cmdlifecycle.SessionState{
		History:  file.History,
		InFlight: file.InFlight,
	}
	if err := cmdSvc.Restore(sessID, state); err != nil {
		return fmt.Errorf("RestoreCommandsForSession: %w", err)
	}
	return nil
}
