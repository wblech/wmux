package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failWriter is an io.Writer that always returns an error.
type failWriter struct{ err error }

func (f *failWriter) Write(_ []byte) (int, error) { return 0, f.err }

// truncatedReader returns only the first n bytes from data, then EOF.
type truncatedReader struct {
	data []byte
	pos  int
	max  int
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if r.pos >= r.max {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:r.max])
	r.pos += n
	return n, nil
}

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

func TestCodec_Encode_HeaderWriteError(t *testing.T) {
	c := Codec{}
	f := Frame{
		Version: ProtocolVersion,
		Type:    MsgData,
		Payload: []byte("data"),
	}

	writeErr := errors.New("disk full")
	err := c.Encode(&failWriter{err: writeErr}, f)
	require.Error(t, err)
	assert.ErrorIs(t, err, writeErr)
}

func TestCodec_Encode_PayloadWriteError(t *testing.T) {
	c := Codec{}
	f := Frame{
		Version: ProtocolVersion,
		Type:    MsgData,
		Payload: []byte("payload data"),
	}

	// A writer that succeeds on the first write (header) but fails on the second (payload).
	writeErr := errors.New("payload write failed")
	w := &partialWriter{writes: 0, failAfter: 1, err: writeErr}
	err := c.Encode(w, f)
	require.Error(t, err)
	assert.ErrorIs(t, err, writeErr)
}

// partialWriter allows n successful writes before returning an error.
type partialWriter struct {
	writes    int
	failAfter int
	err       error
}

func (pw *partialWriter) Write(p []byte) (int, error) {
	if pw.writes >= pw.failAfter {
		return 0, pw.err
	}
	pw.writes++
	return len(p), nil
}

func TestCodec_Decode_EmptyReader(t *testing.T) {
	c := Codec{}
	_, err := c.Decode(bytes.NewReader(nil))
	assert.ErrorIs(t, err, ErrShortRead)
}

func TestCodec_Decode_PayloadTooLargeInHeader(t *testing.T) {
	c := Codec{}

	// Build a header declaring a payload size larger than MaxPayloadSize.
	header := [HeaderSize]byte{
		ProtocolVersion,
		byte(MsgData),
	}
	// Encode MaxPayloadSize+1 as big-endian uint32.
	size := uint32(MaxPayloadSize + 1)
	header[2] = byte(size >> 24)
	header[3] = byte(size >> 16)
	header[4] = byte(size >> 8)
	header[5] = byte(size)

	_, err := c.Decode(bytes.NewReader(header[:]))
	assert.ErrorIs(t, err, ErrPayloadTooLarge)
}

func TestCodec_Decode_ShortPayload(t *testing.T) {
	c := Codec{}

	// Encode a valid frame with a 10-byte payload.
	var buf bytes.Buffer
	f := Frame{
		Version: ProtocolVersion,
		Type:    MsgData,
		Payload: make([]byte, 10),
	}
	require.NoError(t, c.Encode(&buf, f))

	// Provide only the header + 5 bytes of the 10-byte payload.
	truncated := &truncatedReader{data: buf.Bytes(), pos: 0, max: HeaderSize + 5}
	_, err := c.Decode(truncated)
	assert.ErrorIs(t, err, ErrShortRead)
}
