package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuffer_WriteAndRead(t *testing.T) {
	buf := newBuffer(100, 50)
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	got := buf.Read()
	assert.Equal(t, []byte("hello"), got)
}

func TestBuffer_ReadDrains(t *testing.T) {
	buf := newBuffer(100, 50)
	_, err := buf.Write([]byte("data"))
	require.NoError(t, err)

	first := buf.Read()
	assert.Equal(t, []byte("data"), first)

	second := buf.Read()
	assert.Nil(t, second)
}

func TestBuffer_HighWatermarkPause(t *testing.T) {
	buf := newBuffer(10, 5)
	assert.False(t, buf.Paused())

	_, err := buf.Write([]byte("0123456789")) // exactly 10 bytes == high watermark
	require.NoError(t, err)
	assert.True(t, buf.Paused())
}

func TestBuffer_LowWatermarkResume(t *testing.T) {
	buf := newBuffer(10, 5)
	_, err := buf.Write([]byte("0123456789")) // trigger pause
	require.NoError(t, err)
	assert.True(t, buf.Paused())

	_ = buf.Read() // drain all
	assert.False(t, buf.Paused())
}

func TestBuffer_PartialDrainStaysPaused(t *testing.T) {
	buf := newBuffer(10, 5)
	_, err := buf.Write([]byte("0123456789")) // 10 bytes -> paused
	require.NoError(t, err)
	assert.True(t, buf.Paused())

	// Read only 4 bytes; 6 remain, which is still >= lowWatermark (5)
	_ = buf.ReadN(4)
	assert.True(t, buf.Paused())
}

func TestBuffer_DrainBelowLowResumes(t *testing.T) {
	buf := newBuffer(10, 5)
	_, err := buf.Write([]byte("0123456789")) // 10 bytes -> paused
	require.NoError(t, err)
	assert.True(t, buf.Paused())

	// Read 7 bytes; 3 remain, which is below lowWatermark (5) -> should resume
	_ = buf.ReadN(7)
	assert.False(t, buf.Paused())
}

func TestBuffer_MultipleWrites(t *testing.T) {
	buf := newBuffer(100, 50)
	_, err := buf.Write([]byte("foo"))
	require.NoError(t, err)
	_, err = buf.Write([]byte("bar"))
	require.NoError(t, err)
	_, err = buf.Write([]byte("baz"))
	require.NoError(t, err)

	got := buf.Read()
	assert.Equal(t, []byte("foobarbaz"), got)
}

func TestBuffer_ReadN_ExactLength(t *testing.T) {
	buf := newBuffer(100, 50)
	_, err := buf.Write([]byte("hello"))
	require.NoError(t, err)

	// n == len(data): should return all bytes and drain the buffer.
	out := buf.ReadN(5)
	assert.Equal(t, []byte("hello"), out)
	assert.Equal(t, 0, buf.Len())
}

func TestBuffer_ReadN_GreaterThanLength(t *testing.T) {
	buf := newBuffer(100, 50)
	_, err := buf.Write([]byte("hi"))
	require.NoError(t, err)

	// n > len(data): should return all available bytes.
	out := buf.ReadN(100)
	assert.Equal(t, []byte("hi"), out)
	assert.Equal(t, 0, buf.Len())
}

func TestBuffer_ReadN_Empty(t *testing.T) {
	buf := newBuffer(100, 50)

	// ReadN on empty buffer returns nil.
	out := buf.ReadN(10)
	assert.Nil(t, out)
}

func TestBuffer_Len(t *testing.T) {
	buf := newBuffer(100, 50)
	assert.Equal(t, 0, buf.Len())

	_, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, buf.Len())

	_ = buf.ReadN(3)
	assert.Equal(t, 2, buf.Len())

	_ = buf.Read()
	assert.Equal(t, 0, buf.Len())
}
