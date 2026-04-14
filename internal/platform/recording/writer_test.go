package recording

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriter_CreatesAsciinemaV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewWriter(path, 80, 24, 0)
	require.NoError(t, err)

	_, err = w.Write([]byte("hello"))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	// First line is the header.
	var header map[string]any
	err = json.Unmarshal([]byte(lines[0]), &header)
	require.NoError(t, err)
	assert.Equal(t, float64(2), header["version"])
	assert.Equal(t, float64(80), header["width"])
	assert.Equal(t, float64(24), header["height"])

	// Second line is an event: [timestamp, "o", "hello"]
	var event []any
	err = json.Unmarshal([]byte(lines[1]), &event)
	require.NoError(t, err)
	require.Len(t, event, 3)
	assert.Equal(t, "o", event[1])
	assert.Equal(t, "hello", event[2])

	ts, ok := event[0].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, ts, float64(0))
}

func TestWriter_MultipleWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewWriter(path, 80, 24, 0)
	require.NoError(t, err)

	_, err = w.Write([]byte("first"))
	require.NoError(t, err)
	_, err = w.Write([]byte("second"))
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 3)
}

func TestWriter_MaxSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewWriter(path, 80, 24, 200)
	require.NoError(t, err)

	var limitErr error
	for i := 0; i < 100; i++ {
		_, limitErr = w.Write([]byte("data data data data data data data data\n"))
		if limitErr != nil {
			break
		}
	}

	assert.ErrorIs(t, limitErr, ErrSizeLimitReached)
	_ = w.Close()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.LessOrEqual(t, info.Size(), int64(400))
}

func TestWriter_WriteToClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewWriter(path, 80, 24, 0)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	_, err = w.Write([]byte("late"))
	assert.ErrorIs(t, err, ErrRecordingClosed)
}

func TestWriter_Path(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewWriter(path, 80, 24, 0)
	require.NoError(t, err)
	defer w.Close()

	assert.Equal(t, path, w.Path())
}

func TestWriter_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.cast")

	w, err := NewWriter(path, 80, 24, 0)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	err = w.Close()
	assert.NoError(t, err)
}
