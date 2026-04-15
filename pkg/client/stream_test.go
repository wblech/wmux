package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeDataPayload(t *testing.T) {
	t.Run("valid payload", func(t *testing.T) {
		// Encode: [id_len:1][id:2][data:5] = "s1" + "hello"
		payload := []byte{2, 's', '1', 'h', 'e', 'l', 'l', 'o'}
		sessionID, data, err := decodeDataPayload(payload)
		require.NoError(t, err)
		assert.Equal(t, "s1", sessionID)
		assert.Equal(t, []byte("hello"), data)
	})

	t.Run("empty payload", func(t *testing.T) {
		_, _, err := decodeDataPayload(nil)
		require.Error(t, err)
	})

	t.Run("truncated id", func(t *testing.T) {
		// Says id is 5 bytes but only 2 follow
		payload := []byte{5, 'a', 'b'}
		_, _, err := decodeDataPayload(payload)
		require.Error(t, err)
	})

	t.Run("id only no data", func(t *testing.T) {
		payload := []byte{2, 's', '1'}
		sessionID, data, err := decodeDataPayload(payload)
		require.NoError(t, err)
		assert.Equal(t, "s1", sessionID)
		assert.Empty(t, data)
	})
}
