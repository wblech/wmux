package daemon

import "testing"

// BenchmarkEncodeDataPayload measures the cost of framing a session-id +
// data payload for a MsgData broadcast. Baseline: 1 alloc (make).
func BenchmarkEncodeDataPayload(b *testing.B) {
	const chunkSize = 32 * 1024

	sessionID := "sess-abcdef0123456789"
	data := make([]byte, chunkSize)

	b.SetBytes(int64(chunkSize))
	b.ReportAllocs()
	b.ResetTimer()

	var sink []byte
	for i := 0; i < b.N; i++ {
		sink = EncodeDataPayload(sessionID, data)
	}
	_ = sink
}

// BenchmarkAcquireDataPayload measures the pooled equivalent of
// EncodeDataPayload. Acquire+release per call mirrors the broadcast loop.
func BenchmarkAcquireDataPayload(b *testing.B) {
	const chunkSize = 32 * 1024

	sessionID := "sess-abcdef0123456789"
	data := make([]byte, chunkSize)

	b.SetBytes(int64(chunkSize))
	b.ReportAllocs()
	b.ResetTimer()

	var sinkLen int
	for i := 0; i < b.N; i++ {
		payload, handle := AcquireDataPayload(sessionID, data)
		sinkLen = len(payload)
		ReleaseDataPayload(handle)
	}
	_ = sinkLen
}
