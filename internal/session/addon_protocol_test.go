package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeAddonRequest_Create(t *testing.T) {
	frame := EncodeAddonRequest(AddonMethodCreate, "sess-1", []byte(`{"cols":80,"rows":24}`))

	assert.Equal(t, byte(AddonMethodCreate), frame[0])
	assert.Equal(t, byte(6), frame[1]) // len("sess-1")
	assert.Equal(t, "sess-1", string(frame[2:8]))
	payloadLen := int(frame[8])<<24 | int(frame[9])<<16 | int(frame[10])<<8 | int(frame[11])
	assert.Equal(t, 21, payloadLen)
	assert.Equal(t, `{"cols":80,"rows":24}`, string(frame[12:]))
}

func TestEncodeAddonRequest_Process(t *testing.T) {
	data := []byte("hello terminal output")
	frame := EncodeAddonRequest(AddonMethodProcess, "s1", data)

	assert.Equal(t, byte(AddonMethodProcess), frame[0])
	assert.Equal(t, byte(2), frame[1])
	assert.Equal(t, "s1", string(frame[2:4]))
	payloadLen := int(frame[4])<<24 | int(frame[5])<<16 | int(frame[6])<<8 | int(frame[7])
	assert.Equal(t, len(data), payloadLen)
	assert.Equal(t, data, frame[8:])
}

func TestEncodeAddonRequest_Shutdown(t *testing.T) {
	frame := EncodeAddonRequest(AddonMethodShutdown, "", nil)

	assert.Equal(t, byte(AddonMethodShutdown), frame[0])
	assert.Equal(t, byte(0), frame[1])
	assert.Equal(t, []byte{0, 0, 0, 0}, frame[2:6])
}

func TestDecodeAddonResponse_OK(t *testing.T) {
	resp := []byte{
		0x01,       // method
		2,          // session_id_len
		's', '1',   // session_id
		0x00,       // status ok
		0, 0, 0, 0, // payload_len = 0
	}
	method, sessID, status, payload, err := DecodeAddonResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, AddonMethodCreate, method)
	assert.Equal(t, "s1", sessID)
	assert.Equal(t, AddonStatusOK, status)
	assert.Empty(t, payload)
}

func TestDecodeAddonResponse_WithPayload(t *testing.T) {
	scrollback := []byte("scrollback data")
	viewport := []byte("viewport data")
	snapshotPayload := EncodeSnapshotPayload(scrollback, viewport)

	resp := make([]byte, 0)
	resp = append(resp, 0x03)
	resp = append(resp, 2)
	resp = append(resp, 's', '1')
	resp = append(resp, 0x00)
	pLen := len(snapshotPayload)
	resp = append(resp, byte(pLen>>24), byte(pLen>>16), byte(pLen>>8), byte(pLen))
	resp = append(resp, snapshotPayload...)

	method, sessID, status, payload, err := DecodeAddonResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, AddonMethodSnapshot, method)
	assert.Equal(t, "s1", sessID)
	assert.Equal(t, AddonStatusOK, status)

	sb, vp, err := DecodeSnapshotPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, scrollback, sb)
	assert.Equal(t, viewport, vp)
}

func TestDecodeAddonResponse_Error(t *testing.T) {
	resp := []byte{0x01, 2, 's', '1', 0x01, 0, 0, 0, 0}
	_, _, status, _, err := DecodeAddonResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, AddonStatusError, status)
}

func TestDecodeAddonResponse_TooShort(t *testing.T) {
	_, _, _, _, err := DecodeAddonResponse([]byte{0x01})
	assert.Error(t, err)
}

func TestEncodeDecodeSnapshotPayload(t *testing.T) {
	scrollback := []byte("line1\nline2\nline3")
	viewport := []byte("\x1b[1;1Hscreen content")

	encoded := EncodeSnapshotPayload(scrollback, viewport)
	sb, vp, err := DecodeSnapshotPayload(encoded)
	require.NoError(t, err)
	assert.Equal(t, scrollback, sb)
	assert.Equal(t, viewport, vp)
}

func TestDecodeSnapshotPayload_Empty(t *testing.T) {
	sb, vp, err := DecodeSnapshotPayload(nil)
	require.NoError(t, err)
	assert.Empty(t, sb)
	assert.Empty(t, vp)
}
