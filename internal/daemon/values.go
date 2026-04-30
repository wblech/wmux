package daemon

import (
	"errors"
	"sync"
)

// ErrPayloadTooShort is returned when a binary payload cannot be decoded.
var ErrPayloadTooShort = errors.New("daemon: payload too short")

// dataPayloadPool reuses payload buffers across MsgData broadcast frames.
// The slice returned by AcquireDataPayload is borrowed from this pool — its
// backing array MAY be recycled by a later Acquire, so retaining it past
// ReleaseDataPayload is a silent data-corruption bug.
//
// Contract:
//
//	WRONG:  payload, h := AcquireDataPayload(...)
//	        go server.Broadcast(payload)             // outlives Release
//	        ReleaseDataPayload(h)
//
//	OK:     payload, h := AcquireDataPayload(...)
//	        server.Broadcast(payload)                // synchronous
//	        ReleaseDataPayload(h)
//
// Codec.Encode is synchronous (w.Write copies), so releasing after the
// broadcast loop is safe. Regression gate: BenchmarkAcquireDataPayload must
// report 0 allocs/op. If it regresses to >0, the pool stopped working and
// any retained-slice bug in callers stops being silent (they now alias fresh
// allocations and look "fine") — fix the pool, do not relax the bench.
var dataPayloadPool = sync.Pool{
	New: func() any {
		s := make([]byte, 0, 64*1024)
		return &s
	},
}

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

// AcquireDataPayload returns a pooled buffer encoded with the same layout as
// EncodeDataPayload. The returned slice is borrowed from a pool — callers
// MUST call ReleaseDataPayload(handle) when done (typically after broadcast).
// Encode is synchronous so releasing after the broadcast loop is safe.
//
// The handle is the *[]byte taken from the pool; pass it back to release.
func AcquireDataPayload(sessionID string, data []byte) (payload []byte, handle *[]byte) {
	bufPtr := dataPayloadPool.Get().(*[]byte)
	buf := (*bufPtr)[:0]
	buf = append(buf, byte(len(sessionID)))
	buf = append(buf, sessionID...)
	buf = append(buf, data...)
	*bufPtr = buf
	return buf, bufPtr
}

// ReleaseDataPayload returns the buffer acquired via AcquireDataPayload to
// the pool. Safe to call with nil; callers should still avoid passing nil.
func ReleaseDataPayload(handle *[]byte) {
	if handle == nil {
		return
	}
	*handle = (*handle)[:0]
	dataPayloadPool.Put(handle)
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
