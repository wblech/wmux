package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader is an io.Reader that always returns an error, used to simulate
// rand.Reader failures in tests.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func TestGenerate(t *testing.T) {
	token, err := Generate()
	require.NoError(t, err)
	assert.Len(t, token, TokenSize)

	token2, err := Generate()
	require.NoError(t, err)
	assert.NotEqual(t, token, token2)
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.token")

	token, err := Generate()
	require.NoError(t, err)

	err = SaveToFile(token, path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	loaded, err := LoadFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, token, loaded)
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/token")
	assert.Error(t, err)
}

func TestLoadFromFile_WrongSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.token")
	err := os.WriteFile(path, []byte("too-short"), 0o600)
	require.NoError(t, err)

	_, err = LoadFromFile(path)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerify_Match(t *testing.T) {
	token, err := Generate()
	require.NoError(t, err)

	candidate := make([]byte, TokenSize)
	copy(candidate, token)

	assert.True(t, Verify(token, candidate))
}

func TestVerify_Mismatch(t *testing.T) {
	token, err := Generate()
	require.NoError(t, err)

	bad := make([]byte, TokenSize)
	assert.False(t, Verify(token, bad))
}

func TestVerify_WrongLength(t *testing.T) {
	token, err := Generate()
	require.NoError(t, err)

	assert.False(t, Verify(token, []byte("short")))
}

func TestEnsure_GeneratesNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.token")

	token, err := Ensure(path)
	require.NoError(t, err)
	assert.Len(t, token, TokenSize)

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestEnsure_LoadsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.token")

	original, err := Generate()
	require.NoError(t, err)
	err = SaveToFile(original, path)
	require.NoError(t, err)

	loaded, err := Ensure(path)
	require.NoError(t, err)
	assert.Equal(t, original, loaded)
}

func TestGenerate_RandError(t *testing.T) {
	orig := randReader
	randReader = errReader{}

	t.Cleanup(func() { randReader = orig })

	_, err := Generate()
	assert.Error(t, err)
}

func TestEnsure_GenerateError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")

	orig := randReader
	randReader = errReader{}

	t.Cleanup(func() { randReader = orig })

	_, err := Ensure(path)
	assert.Error(t, err)
}

func TestEnsure_SaveError(t *testing.T) {
	// Point to a path inside a read-only directory so SaveToFile fails.
	dir := t.TempDir()

	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip("cannot change directory permissions")
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "token")
	_, err := Ensure(path)
	assert.Error(t, err)
}

func TestSaveToFile_InvalidPath(t *testing.T) {
	err := SaveToFile([]byte("data"), "/nonexistent/dir/token")
	assert.Error(t, err)
}

func TestEnsure_ErrorNotPermitted(t *testing.T) {
	// Use a path under a directory with no write permission so
	// LoadFromFile returns a permission-denied error (not ErrNotExist
	// nor ErrInvalidToken), exercising the generic error branch in Ensure.
	dir := t.TempDir()

	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skip("cannot change directory permissions")
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "token")
	_, err := Ensure(path)
	assert.Error(t, err)
}

func TestEnsure_RegeneratesInvalidToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.token")
	err := os.WriteFile(path, []byte("too-short"), 0o600)
	require.NoError(t, err)

	token, err := Ensure(path)
	require.NoError(t, err)
	assert.Len(t, token, TokenSize)
}
