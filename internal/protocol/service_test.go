package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodec_RoundTrip(t *testing.T) {
	c := Codec{}
	original := Frame{
		Version: ProtocolVersion,
		Type:    MsgData,
		Payload: []byte("hello, wmux"),
	}

	var buf bytes.Buffer
	err := c.Encode(&buf, original)
	require.NoError(t, err)

	decoded, err := c.Decode(&buf)
	require.NoError(t, err)

	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, original.Payload, decoded.Payload)
}

func TestCodec_EmptyPayload(t *testing.T) {
	c := Codec{}
	f := Frame{
		Version: ProtocolVersion,
		Type:    MsgHeartbeat,
		Payload: nil,
	}

	var buf bytes.Buffer
	err := c.Encode(&buf, f)
	require.NoError(t, err)

	assert.Equal(t, HeaderSize, buf.Len())
}

func TestCodec_MultipleFrames(t *testing.T) {
	c := Codec{}
	frames := []Frame{
		{Version: ProtocolVersion, Type: MsgCreate, Payload: []byte("session-1")},
		{Version: ProtocolVersion, Type: MsgAttach, Payload: []byte("session-2")},
		{Version: ProtocolVersion, Type: MsgDetach, Payload: []byte("session-3")},
	}

	var buf bytes.Buffer
	for _, f := range frames {
		require.NoError(t, c.Encode(&buf, f))
	}

	for _, want := range frames {
		got, err := c.Decode(&buf)
		require.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.Equal(t, want.Payload, got.Payload)
	}
}

func TestCodec_VersionMismatch(t *testing.T) {
	c := Codec{}
	f := Frame{
		Version: 99,
		Type:    MsgData,
		Payload: []byte("bad version"),
	}

	var buf bytes.Buffer
	require.NoError(t, c.Encode(&buf, f))

	_, err := c.Decode(&buf)
	assert.ErrorIs(t, err, ErrVersionMismatch)
}

func TestCodec_PayloadTooLarge(t *testing.T) {
	c := Codec{}
	f := Frame{
		Version: ProtocolVersion,
		Type:    MsgData,
		Payload: make([]byte, MaxPayloadSize+1),
	}

	var buf bytes.Buffer
	err := c.Encode(&buf, f)
	assert.ErrorIs(t, err, ErrPayloadTooLarge)
}

func TestCodec_ShortRead(t *testing.T) {
	c := Codec{}
	buf := bytes.NewReader([]byte{0x01, 0x01}) // only 2 bytes, not a full header

	_, err := c.Decode(buf)
	assert.ErrorIs(t, err, ErrShortRead)
}
