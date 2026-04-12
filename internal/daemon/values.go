package daemon

import "errors"

// ErrPayloadTooShort is returned when a binary payload cannot be decoded.
var ErrPayloadTooShort = errors.New("daemon: payload too short")

// EncodeDataPayload encodes a session ID and data into a binary payload
// for MsgData frames: [session_id_len:1][session_id:N][data:rest].
func EncodeDataPayload(sessionID string, data []byte) []byte {
	idBytes := []byte(sessionID)
	payload := make([]byte, 0, 1+len(idBytes)+len(data))
	payload = append(payload, byte(len(idBytes)))
	payload = append(payload, idBytes...)
	payload = append(payload, data...)
	return payload
}

// DecodeDataPayload decodes a binary payload from a MsgData frame.
func DecodeDataPayload(payload []byte) (sessionID string, data []byte, err error) {
	if len(payload) < 1 {
		return "", nil, ErrPayloadTooShort
	}
	idLen := int(payload[0])
	if len(payload) < 1+idLen {
		return "", nil, ErrPayloadTooShort
	}
	sessionID = string(payload[1 : 1+idLen])
	data = payload[1+idLen:]
	return sessionID, data, nil
}

// EncodeInputPayload encodes a session ID and input data into a binary
// payload for MsgInput frames. Same format as MsgData.
func EncodeInputPayload(sessionID string, data []byte) []byte {
	return EncodeDataPayload(sessionID, data)
}

// DecodeInputPayload decodes a binary payload from a MsgInput frame.
func DecodeInputPayload(payload []byte) (sessionID string, data []byte, err error) {
	return DecodeDataPayload(payload)
}
