package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSessionDir(t *testing.T) {
	base := t.TempDir()

	dir, err := EnsureSessionDir(base, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "sess-1"), dir)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureSessionDir_Idempotent(t *testing.T) {
	base := t.TempDir()

	dir1, err := EnsureSessionDir(base, "sess-x")
	require.NoError(t, err)

	dir2, err := EnsureSessionDir(base, "sess-x")
	require.NoError(t, err)
	assert.Equal(t, dir1, dir2)
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 100), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.bin"), make([]byte, 200), 0644))

	size, err := DirSize(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(300), size)
}

func TestListSessionDirs(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "s1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	end1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d1, Metadata{
		SessionID: "s1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end1,
		ExitCode:  intPtr(0),
	}))

	d2 := filepath.Join(base, "s2")
	require.NoError(t, os.MkdirAll(d2, 0755))
	end2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d2, Metadata{
		SessionID: "s2",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end2,
		ExitCode:  intPtr(0),
	}))

	d3 := filepath.Join(base, "s3")
	require.NoError(t, os.MkdirAll(d3, 0755))
	require.NoError(t, WriteMetadata(d3, Metadata{
		SessionID: "s3",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   nil,
		ExitCode:  nil,
	}))

	dirs, err := ListSessionDirs(base)
	require.NoError(t, err)
	require.Len(t, dirs, 3)
}

func TestTotalSize(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "s1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(d1, "scrollback.bin"), make([]byte, 500), 0644))

	d2 := filepath.Join(base, "s2")
	require.NoError(t, os.MkdirAll(d2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "scrollback.bin"), make([]byte, 300), 0644))

	total, err := TotalSize(base)
	require.NoError(t, err)
	assert.Equal(t, int64(800), total)
}

func TestEvictLRU_RemovesOldestCompleted(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "old")
	require.NoError(t, os.MkdirAll(d1, 0755))
	end1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d1, Metadata{
		SessionID: "old",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end1,
		ExitCode:  intPtr(0),
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d1, "scrollback.bin"), make([]byte, 500), 0644))

	d2 := filepath.Join(base, "new")
	require.NoError(t, os.MkdirAll(d2, 0755))
	end2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d2, Metadata{
		SessionID: "new",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end2,
		ExitCode:  intPtr(0),
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "scrollback.bin"), make([]byte, 300), 0644))

	d3 := filepath.Join(base, "active")
	require.NoError(t, os.MkdirAll(d3, 0755))
	require.NoError(t, WriteMetadata(d3, Metadata{
		SessionID: "active",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   nil,
		ExitCode:  nil,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d3, "scrollback.bin"), make([]byte, 200), 0644))

	// Evict until total <= 900. Should remove "old" (675B including meta.json, oldest endedAt).
	err := EvictLRU(base, 900)
	require.NoError(t, err)

	_, err = os.Stat(d1)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(d2)
	require.NoError(t, err)
	_, err = os.Stat(d3)
	require.NoError(t, err)
}

func TestEvictLRU_SkipsActiveSessions(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "active1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	require.NoError(t, WriteMetadata(d1, Metadata{
		SessionID: "active1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   nil,
		ExitCode:  nil,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d1, "scrollback.bin"), make([]byte, 500), 0644))

	err := EvictLRU(base, 100)
	require.NoError(t, err)

	_, err = os.Stat(d1)
	require.NoError(t, err)
}

func TestEvictLRU_NoOpUnderLimit(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "s1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	end1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d1, Metadata{
		SessionID: "s1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end1,
		ExitCode:  intPtr(0),
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d1, "scrollback.bin"), make([]byte, 100), 0644))

	err := EvictLRU(base, 1000)
	require.NoError(t, err)

	_, err = os.Stat(d1)
	require.NoError(t, err)
}

func TestEnsureSessionDir_Error(t *testing.T) {
	// Try to create a directory in a non-existent parent with no permissions
	_, err := EnsureSessionDir("/dev/null/invalid", "sess-1")
	assert.Error(t, err)
}

func TestDirSize_Empty(t *testing.T) {
	dir := t.TempDir()

	size, err := DirSize(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(0), size)
}

func TestListSessionDirs_Empty(t *testing.T) {
	base := t.TempDir()

	dirs, err := ListSessionDirs(base)
	require.NoError(t, err)
	require.Empty(t, dirs)
}

func TestListSessionDirs_SkipsNoMeta(t *testing.T) {
	base := t.TempDir()

	// Create a directory without meta.json
	noMetaDir := filepath.Join(base, "no-meta")
	require.NoError(t, os.MkdirAll(noMetaDir, 0755))

	dirs, err := ListSessionDirs(base)
	require.NoError(t, err)
	require.Empty(t, dirs)
}

func TestTotalSize_Empty(t *testing.T) {
	base := t.TempDir()

	total, err := TotalSize(base)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
}

func TestEvictLRU_NoCompleted(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "active1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	require.NoError(t, WriteMetadata(d1, Metadata{
		SessionID: "active1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   nil,
		ExitCode:  nil,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d1, "scrollback.bin"), make([]byte, 500), 0644))

	d2 := filepath.Join(base, "active2")
	require.NoError(t, os.MkdirAll(d2, 0755))
	require.NoError(t, WriteMetadata(d2, Metadata{
		SessionID: "active2",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   nil,
		ExitCode:  nil,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "scrollback.bin"), make([]byte, 300), 0644))

	// Should not evict any active sessions even with low limit
	err := EvictLRU(base, 100)
	require.NoError(t, err)

	_, err = os.Stat(d1)
	require.NoError(t, err)
	_, err = os.Stat(d2)
	require.NoError(t, err)
}

func TestEvictLRU_MultipleEvictions(t *testing.T) {
	base := t.TempDir()

	d1 := filepath.Join(base, "1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	end1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d1, Metadata{
		SessionID: "1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end1,
		ExitCode:  intPtr(0),
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d1, "f"), make([]byte, 200), 0644))

	d2 := filepath.Join(base, "2")
	require.NoError(t, os.MkdirAll(d2, 0755))
	end2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d2, Metadata{
		SessionID: "2",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end2,
		ExitCode:  intPtr(0),
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d2, "f"), make([]byte, 200), 0644))

	d3 := filepath.Join(base, "3")
	require.NoError(t, os.MkdirAll(d3, 0755))
	end3 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, WriteMetadata(d3, Metadata{
		SessionID: "3",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now(),
		EndedAt:   &end3,
		ExitCode:  intPtr(0),
	}))
	require.NoError(t, os.WriteFile(filepath.Join(d3, "f"), make([]byte, 200), 0644))

	// Should remove both 1 and 2 to get to 500 or below
	err := EvictLRU(base, 500)
	require.NoError(t, err)

	_, err = os.Stat(d1)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(d2)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(d3)
	require.NoError(t, err)
}

func intPtr(n int) *int {
	return &n
}
