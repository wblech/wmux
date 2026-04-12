package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteMetadata(t *testing.T) {
	dir := t.TempDir()

	meta := Metadata{
		SessionID: "test-1",
		Shell:     "/bin/sh",
		Cwd:       "/home/user",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		EndedAt:   nil,
		ExitCode:  nil,
	}

	err := WriteMetadata(dir, meta)
	require.NoError(t, err)

	// File should exist.
	_, err = os.Stat(filepath.Join(dir, "meta.json"))
	assert.NoError(t, err)
}

func TestReadMetadata(t *testing.T) {
	dir := t.TempDir()

	meta := Metadata{
		SessionID: "test-2",
		Shell:     "/bin/zsh",
		Cwd:       "/tmp",
		Cols:      120,
		Rows:      40,
		StartedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		EndedAt:   nil,
		ExitCode:  nil,
	}

	err := WriteMetadata(dir, meta)
	require.NoError(t, err)

	got, err := ReadMetadata(dir)
	require.NoError(t, err)
	assert.Equal(t, "test-2", got.SessionID)
	assert.Equal(t, "/bin/zsh", got.Shell)
	assert.Equal(t, "/tmp", got.Cwd)
	assert.Equal(t, 120, got.Cols)
	assert.Equal(t, 40, got.Rows)
	assert.True(t, got.StartedAt.Equal(meta.StartedAt))
	assert.Nil(t, got.EndedAt)
	assert.Nil(t, got.ExitCode)
}

func TestUpdateMetadataExit(t *testing.T) {
	dir := t.TempDir()

	meta := Metadata{
		SessionID: "test-3",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		EndedAt:   nil,
		ExitCode:  nil,
	}

	err := WriteMetadata(dir, meta)
	require.NoError(t, err)

	endTime := time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC)

	err = UpdateMetadataExit(dir, endTime, 0)
	require.NoError(t, err)

	got, err := ReadMetadata(dir)
	require.NoError(t, err)
	assert.NotNil(t, got.EndedAt)
	assert.True(t, got.EndedAt.Equal(endTime))
	assert.NotNil(t, got.ExitCode)
	assert.Equal(t, 0, *got.ExitCode)
}

func TestUpdateMetadataExit_NonZeroCode(t *testing.T) {
	dir := t.TempDir()

	meta := Metadata{
		SessionID: "test-4",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		EndedAt:   nil,
		ExitCode:  nil,
	}

	err := WriteMetadata(dir, meta)
	require.NoError(t, err)

	endTime := time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC)

	err = UpdateMetadataExit(dir, endTime, 1)
	require.NoError(t, err)

	got, err := ReadMetadata(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, *got.ExitCode)
}

func TestReadMetadata_NotFound(t *testing.T) {
	_, err := ReadMetadata(t.TempDir())
	assert.Error(t, err)
}

func TestWriteMetadata_InvalidDir(t *testing.T) {
	meta := Metadata{
		SessionID: "x",
		Shell:     "",
		Cwd:       "",
		Cols:      0,
		Rows:      0,
		StartedAt: time.Time{},
		EndedAt:   nil,
		ExitCode:  nil,
	}
	err := WriteMetadata("/nonexistent/path/dir", meta)
	assert.Error(t, err)
}
