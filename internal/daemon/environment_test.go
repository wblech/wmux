package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardEnv_SymlinkForSocket(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "s1")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	sockPath := filepath.Join(dir, "ssh-agent.sock")
	require.NoError(t, os.WriteFile(sockPath, nil, 0600))

	err := ForwardEnv(sessionDir, "SSH_AUTH_SOCK", sockPath)
	require.NoError(t, err)

	symlinkPath := filepath.Join(sessionDir, "SSH_AUTH_SOCK")
	target, err := os.Readlink(symlinkPath)
	require.NoError(t, err)
	assert.Equal(t, sockPath, target)
}

func TestForwardEnv_UpdatesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "s1")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	oldSock := filepath.Join(dir, "old.sock")
	newSock := filepath.Join(dir, "new.sock")
	require.NoError(t, os.WriteFile(oldSock, nil, 0600))
	require.NoError(t, os.WriteFile(newSock, nil, 0600))

	require.NoError(t, ForwardEnv(sessionDir, "SSH_AUTH_SOCK", oldSock))
	require.NoError(t, ForwardEnv(sessionDir, "SSH_AUTH_SOCK", newSock))

	target, err := os.Readlink(filepath.Join(sessionDir, "SSH_AUTH_SOCK"))
	require.NoError(t, err)
	assert.Equal(t, newSock, target)
}

func TestForwardEnv_NonexistentValue(t *testing.T) {
	dir := t.TempDir()
	err := ForwardEnv(dir, "SSH_AUTH_SOCK", "/no/such/path")
	assert.Error(t, err)
}

func TestWriteEnvFile(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions", "s1")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	env := map[string]string{
		"DISPLAY":        ":0",
		"SSH_CONNECTION": "1.2.3.4 22",
	}
	err := WriteEnvFile(sessionDir, env)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(sessionDir, "env"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "DISPLAY=:0")
	assert.Contains(t, content, "SSH_CONNECTION=1.2.3.4 22")
}

func TestWriteEnvFile_Empty(t *testing.T) {
	dir := t.TempDir()
	err := WriteEnvFile(dir, map[string]string{})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "env"))
	require.NoError(t, err)
	assert.Equal(t, "\n", string(data))
}
