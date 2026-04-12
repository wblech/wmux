package history

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SessionDirInfo holds info about a session's history directory.
type SessionDirInfo struct {
	// Path is the absolute path to the session directory.
	Path string
	// SessionID is the session identifier (directory name).
	SessionID string
	// EndedAt is non-nil for completed sessions.
	EndedAt *time.Time
	// Size is the total size in bytes of all files in the directory.
	Size int64
}

// EnsureSessionDir creates the session directory under baseDir if it doesn't exist.
// Returns the full path to the directory.
func EnsureSessionDir(baseDir, sessionID string) (string, error) {
	dir := filepath.Join(baseDir, sessionID)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("history: ensure session dir: %w", err)
	}

	return dir, nil
}

// DirSize returns the total size in bytes of all regular files in the directory.
// It does not recurse into subdirectories.
func DirSize(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("history: read dir for size: %w", err)
	}

	var total int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		total += info.Size()
	}

	return total, nil
}

// ListSessionDirs reads all session directories under baseDir and returns
// their info. Each subdirectory is expected to contain a meta.json file.
// Directories without meta.json are skipped.
func ListSessionDirs(baseDir string) ([]SessionDirInfo, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, fmt.Errorf("history: list session dirs: %w", err)
	}

	var dirs []SessionDirInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(baseDir, entry.Name())

		meta, err := ReadMetadata(dirPath)
		if err != nil {
			continue
		}

		size, _ := DirSize(dirPath)

		dirs = append(dirs, SessionDirInfo{
			Path:      dirPath,
			SessionID: meta.SessionID,
			EndedAt:   meta.EndedAt,
			Size:      size,
		})
	}

	return dirs, nil
}

// TotalSize returns the sum of all file sizes across all session directories.
func TotalSize(baseDir string) (int64, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0, fmt.Errorf("history: total size: %w", err)
	}

	var total int64

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		size, _ := DirSize(filepath.Join(baseDir, entry.Name()))
		total += size
	}

	return total, nil
}

// EvictLRU removes the oldest completed session directories until the total
// disk usage is at or below maxTotal bytes. Active sessions (EndedAt == nil)
// are never evicted. If total is already under the limit, this is a no-op.
func EvictLRU(baseDir string, maxTotal int64) error {
	dirs, err := ListSessionDirs(baseDir)
	if err != nil {
		return fmt.Errorf("history: evict lru: %w", err)
	}

	var currentTotal int64
	for _, d := range dirs {
		currentTotal += d.Size
	}

	if currentTotal <= maxTotal {
		return nil
	}

	var completed []SessionDirInfo

	for _, d := range dirs {
		if d.EndedAt != nil {
			completed = append(completed, d)
		}
	}

	sort.Slice(completed, func(i, j int) bool {
		return completed[i].EndedAt.Before(*completed[j].EndedAt)
	})

	for _, d := range completed {
		if currentTotal <= maxTotal {
			break
		}

		if err := os.RemoveAll(d.Path); err != nil {
			return fmt.Errorf("history: remove %s: %w", d.SessionID, err)
		}

		currentTotal -= d.Size
	}

	return nil
}
