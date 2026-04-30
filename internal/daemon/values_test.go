package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDataPayload(t *testing.T) {
	payload := EncodeDataPayload("my-session", []byte("hello world"))
	sid, data, err := DecodeDataPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, "my-session", sid)
	assert.Equal(t, []byte("hello world"), data)
}

func TestDecodeDataPayload_TooShort(t *testing.T) {
	_, _, err := DecodeDataPayload(nil)
	require.Error(t, err)

	_, _, err = DecodeDataPayload([]byte{5})
	require.Error(t, err)
}

func TestDecodeDataPayload_EmptyData(t *testing.T) {
	payload := EncodeDataPayload("s1", nil)
	sid, data, err := DecodeDataPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, "s1", sid)
	assert.Empty(t, data)
}

func TestEncodeInputPayload(t *testing.T) {
	payload := EncodeInputPayload("my-session", []byte("ls\n"))
	sid, data, err := DecodeInputPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, "my-session", sid)
	assert.Equal(t, []byte("ls\n"), data)
}
