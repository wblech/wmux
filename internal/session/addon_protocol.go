package session

import (
	"encoding/binary"
	"errors"
)

// AddonMethod identifies the operation in an addon protocol frame.
type AddonMethod byte

// Addon protocol method constants.
const (
	AddonMethodCreate   AddonMethod = 0x01
	AddonMethodProcess  AddonMethod = 0x02
	AddonMethodSnapshot AddonMethod = 0x03
	AddonMethodResize   AddonMethod = 0x04
	AddonMethodDestroy  AddonMethod = 0x05
	AddonMethodShutdown AddonMethod = 0x06
)

// AddonStatus indicates success or failure in an addon response.
type AddonStatus byte

// Addon protocol status constants.
const (
	AddonStatusOK    AddonStatus = 0x00
	AddonStatusError AddonStatus = 0x01
)

// Sentinel errors for addon protocol operations.
var (
	ErrAddonFrameTooShort    = errors.New("addon: frame too short")
	ErrAddonSnapshotTooShort = errors.New("addon: snapshot payload too short")
	ErrAddonRequestFailed    = errors.New("addon: request failed")
)

// EncodeAddonRequest builds a binary request frame:
// [method:1][session_id_len:1][session_id:N][payload_len:4][payload:N]
func EncodeAddonRequest(method AddonMethod, sessionID string, payload []byte) []byte {
	idBytes := []byte(sessionID)
	frame := make([]byte, 0, 1+1+len(idBytes)+4+len(payload))
	frame = append(frame, byte(method))
	frame = append(frame, byte(len(idBytes)))
	frame = append(frame, idBytes...)
	pLen := make([]byte, 4)
	binary.BigEndian.PutUint32(pLen, uint32(len(payload)))
	frame = append(frame, pLen...)
	frame = append(frame, payload...)
	return frame
}

// DecodeAddonResponse parses a binary response frame:
// [method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:N]
func DecodeAddonResponse(data []byte) (method AddonMethod, sessionID string, status AddonStatus, payload []byte, err error) {
	if len(data) < 3 {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	method = AddonMethod(data[0])
	idLen := int(data[1])
	offset := 2
	if len(data) < offset+idLen+1+4 {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	sessionID = string(data[offset : offset+idLen])
	offset += idLen
	status = AddonStatus(data[offset])
	offset++
	if len(data) < offset+4 {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	pLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	if uint32(len(data)-offset) < pLen {
		return 0, "", 0, nil, ErrAddonFrameTooShort
	}
	payload = data[offset : offset+int(pLen)]
	return method, sessionID, status, payload, nil
}

// EncodeSnapshotPayload builds: [scrollback_len:4][scrollback:N][viewport:N]
func EncodeSnapshotPayload(scrollback, viewport []byte) []byte {
	buf := make([]byte, 4+len(scrollback)+len(viewport))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(scrollback)))
	copy(buf[4:], scrollback)
	copy(buf[4+len(scrollback):], viewport)
	return buf
}

// DecodeSnapshotPayload parses: [scrollback_len:4][scrollback:N][viewport:N]
func DecodeSnapshotPayload(data []byte) (scrollback, viewport []byte, err error) {
	if len(data) == 0 {
		return nil, nil, nil
	}
	if len(data) < 4 {
		return nil, nil, ErrAddonSnapshotTooShort
	}
	sbLen := binary.BigEndian.Uint32(data[:4])
	if uint32(len(data)-4) < sbLen {
		return nil, nil, ErrAddonSnapshotTooShort
	}
	scrollback = data[4 : 4+sbLen]
	viewport = data[4+sbLen:]
	return scrollback, viewport, nil
}
