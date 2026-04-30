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

// BenchmarkParseOSC_NoOSC measures the cost of scanning a typical PTY chunk
// that contains NO OSC sequences (the common case). Baseline allocates the
// whole chunk as a string just to scan it.
func BenchmarkParseOSC_NoOSC(b *testing.B) {
	const chunkSize = 32 * 1024

	data := make([]byte, chunkSize)
	for i := range data {
		data[i] = byte('a' + (i % 26))
	}

	b.SetBytes(int64(chunkSize))
	b.ReportAllocs()
	b.ResetTimer()

	var sinkLen int
	for i := 0; i < b.N; i++ {
		sinkLen = len(ParseOSC(data))
	}
	_ = sinkLen
}

// BenchmarkParseOSC_WithOSC measures the cost when the chunk contains a
// realistic mix of OSC sequences (cwd update + notification).
func BenchmarkParseOSC_WithOSC(b *testing.B) {
	const chunkSize = 32 * 1024

	prefix := []byte("\x1b]7;file:///home/user/some/path\x1b\\hello world ")
	suffix := []byte(" finished\x1b]9;build done\x1b\\")
	mid := make([]byte, chunkSize-len(prefix)-len(suffix))
	for i := range mid {
		mid[i] = byte('a' + (i % 26))
	}
	data := make([]byte, 0, chunkSize)
	data = append(data, prefix...)
	data = append(data, mid...)
	data = append(data, suffix...)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	var sinkLen int
	for i := 0; i < b.N; i++ {
		sinkLen = len(ParseOSC(data))
	}
	_ = sinkLen
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
