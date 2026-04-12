package daemon

import (
	"fmt"
	"time"

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
