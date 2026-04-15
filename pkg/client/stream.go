package client

import "errors"

// errDataPayloadTooShort is returned when a MsgData payload is malformed.
var errDataPayloadTooShort = errors.New("client: data payload too short")

// decodeDataPayload decodes a binary MsgData payload: [id_len:1][id:N][data:rest].
func decodeDataPayload(payload []byte) (sessionID string, data []byte, err error) {
	if len(payload) < 1 {
		return "", nil, errDataPayloadTooShort
	}
	idLen := int(payload[0])
	if len(payload) < 1+idLen {
		return "", nil, errDataPayloadTooShort
	}
	return string(payload[1 : 1+idLen]), payload[1+idLen:], nil
}
