package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAndReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wmux.pid")

	info := Info{
		PID:       12345,
		Version:   "1.0.0",
		StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	err := WritePIDFile(path, info)
	require.NoError(t, err)

	// Check file permissions.
	stat, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), stat.Mode().Perm())

	// Read back and verify fields.
	got, err := ReadPIDFile(path)
	require.NoError(t, err)
	assert.Equal(t, info.PID, got.PID)
	assert.Equal(t, info.Version, got.Version)
	assert.True(t, info.StartedAt.Equal(got.StartedAt))
}

func TestReadPIDFile_NotFound(t *testing.T) {
	_, err := ReadPIDFile("/nonexistent/path/wmux.pid")
	assert.Error(t, err)
}

func TestReadPIDFile_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wmux.pid")

	err := os.WriteFile(path, []byte("not-valid-json"), 0o644)
	require.NoError(t, err)

	_, err = ReadPIDFile(path)
	assert.Error(t, err)
}

func TestRemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wmux.pid")

	err := os.WriteFile(path, []byte("{}"), 0o644)
	require.NoError(t, err)

	err = RemovePIDFile(path)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestRemovePIDFile_NotExists(t *testing.T) {
	err := RemovePIDFile("/nonexistent/path/wmux.pid")
	assert.NoError(t, err)
}

func TestCheckPIDFile_Running(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wmux.pid")

	info := Info{
		PID:       os.Getpid(),
		Version:   "1.0.0",
		StartedAt: time.Now(),
	}

	err := WritePIDFile(path, info)
	require.NoError(t, err)

	running, got, err := CheckPIDFile(path)
	require.NoError(t, err)
	assert.True(t, running)
	assert.Equal(t, info.PID, got.PID)
}

func TestCheckPIDFile_NotRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wmux.pid")

	info := Info{
		PID:       999999999,
		Version:   "1.0.0",
		StartedAt: time.Now(),
	}

	err := WritePIDFile(path, info)
	require.NoError(t, err)

	running, _, err := CheckPIDFile(path)
	require.NoError(t, err)
	assert.False(t, running)
}

func TestCheckPIDFile_NoFile(t *testing.T) {
	running, got, err := CheckPIDFile("/nonexistent/path/wmux.pid")
	require.NoError(t, err)
	assert.False(t, running)
	assert.Equal(t, Info{PID: 0, Version: "", StartedAt: time.Time{}}, got)
}

// Compile-time check that Info fields are JSON-serialisable (covers exhaustruct).
var _ = func() {
	_, _ = json.Marshal(Info{PID: 0, Version: "", StartedAt: time.Time{}})
}
