package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/history"
)

func TestLoadSessionHistory_Success(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "test-session")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	meta := history.Metadata{
		SessionID: "test-session",
		Shell:     "/bin/zsh",
		Cwd:       "/home/user",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now().Add(-time.Hour),
		EndedAt:   nil,
		ExitCode:  nil,
	}
	require.NoError(t, history.WriteMetadata(sessionDir, meta))

	scrollbackPath := filepath.Join(sessionDir, "scrollback.bin")
	require.NoError(t, os.WriteFile(scrollbackPath, []byte("hello world\n"), 0644))

	h, err := LoadSessionHistory(dataDir, "test-session")
	require.NoError(t, err)
	assert.Equal(t, "test-session", h.SessionID)
	assert.Equal(t, "/bin/zsh", h.Shell)
	assert.Equal(t, "/home/user", h.Cwd)
	assert.Equal(t, 80, h.Cols)
	assert.Equal(t, 24, h.Rows)
	assert.Equal(t, []byte("hello world\n"), h.Scrollback)
}

func TestLoadSessionHistory_NoScrollback(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "no-scroll")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	meta := history.Metadata{
		SessionID: "no-scroll",
		Shell:     "/bin/sh",
		Cwd:       "/tmp",
		Cols:      120,
		Rows:      40,
		StartedAt: time.Now().Add(-time.Hour),
		EndedAt:   nil,
		ExitCode:  nil,
	}
	require.NoError(t, history.WriteMetadata(sessionDir, meta))

	h, err := LoadSessionHistory(dataDir, "no-scroll")
	require.NoError(t, err)
	assert.Equal(t, "no-scroll", h.SessionID)
	assert.Nil(t, h.Scrollback)
}

func TestLoadSessionHistory_NotFound(t *testing.T) {
	dataDir := t.TempDir()
	_, err := LoadSessionHistory(dataDir, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrColdRestoreNotAvailable)
}

func TestLoadSessionHistory_CleanExit(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "test-session")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	now := time.Now()
	exitCode := 0
	meta := history.Metadata{
		SessionID: "test-session",
		Shell:     "/bin/zsh",
		Cwd:       "/home/user",
		Cols:      80,
		Rows:      24,
		StartedAt: now.Add(-time.Hour),
		EndedAt:   &now,
		ExitCode:  &exitCode,
	}
	require.NoError(t, history.WriteMetadata(sessionDir, meta))

	_, err := LoadSessionHistory(dataDir, "test-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrColdRestoreNotAvailable)
}

func TestCleanSessionHistory(t *testing.T) {
	dataDir := t.TempDir()
	sessionDir := filepath.Join(dataDir, "test-session")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "meta.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionDir, "scrollback.bin"), []byte("data"), 0644))

	err := CleanSessionHistory(dataDir, "test-session")
	require.NoError(t, err)

	_, err = os.Stat(sessionDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCleanSessionHistory_NotFound(t *testing.T) {
	dataDir := t.TempDir()
	err := CleanSessionHistory(dataDir, "nonexistent")
	assert.NoError(t, err) // idempotent
}
