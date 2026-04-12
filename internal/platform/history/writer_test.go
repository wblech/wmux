package history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriter_BasicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 0) // 0 = unlimited
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	n, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(5), w.Written())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestWriter_MultipleWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 0)
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	_, err = w.Write([]byte("aaa"))
	require.NoError(t, err)
	_, err = w.Write([]byte("bbb"))
	require.NoError(t, err)

	assert.Equal(t, int64(6), w.Written())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "aaabbb", string(data))
}

func TestWriter_CappedAtMaxSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 10) // cap at 10 bytes
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	n, err := w.Write([]byte("12345"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	// This write would exceed the cap: only 5 bytes remain.
	n, err = w.Write([]byte("1234567890"))
	require.NoError(t, err)
	assert.Equal(t, 5, n) // partial write: 5 bytes fit
	assert.Equal(t, int64(10), w.Written())

	// Further writes return 0 (capped).
	n, err = w.Write([]byte("more"))
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestWriter_ExactCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 5)
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	n, err := w.Write([]byte("12345"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	n, err = w.Write([]byte("x"))
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestWriter_CloseAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 0)
	require.NoError(t, err)
	_, _ = w.Write([]byte("first"))
	require.NoError(t, w.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "first", string(data))
}

func TestWriter_WriteAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 0)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	_, err = w.Write([]byte("after"))
	assert.ErrorIs(t, err, ErrWriterClosed)
}

func TestWriter_NewWriterInvalidPath(t *testing.T) {
	_, err := NewWriter("/nonexistent/dir/scrollback.bin", 0)
	assert.Error(t, err)
}

func TestWriter_DoubleClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scrollback.bin")

	w, err := NewWriter(path, 0)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Second close is a no-op, not an error.
	err = w.Close()
	assert.NoError(t, err)
}
