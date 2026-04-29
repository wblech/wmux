package session

import (
	"sync"
	"testing"
	"time"
)

// BenchmarkBatcherAddFlush measures the steady-state cost of pushing a chunk
// through the batcher and waiting for it to be flushed via FlushNow.
// This is the closest microbench to the production hot path:
// readLoop → batcher.Add → (timer or FlushNow) → onFlush → buffer.Write.
func BenchmarkBatcherAddFlush(b *testing.B) {
	const chunkSize = 32 * 1024 // matches readChunkSize

	chunk := make([]byte, chunkSize)
	for i := range chunk {
		chunk[i] = 0x41
	}

	var (
		mu        sync.Mutex
		flushedCh = make(chan struct{}, 1)
	)

	batcher := newBatcher(time.Hour, func(_ []byte) {
		mu.Lock()
		select {
		case flushedCh <- struct{}{}:
		default:
		}
		mu.Unlock()
	})
	defer batcher.Stop()

	b.SetBytes(int64(chunkSize))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		batcher.Add(chunk)
		batcher.FlushNow()
		<-flushedCh
	}
}

// BenchmarkDoFlush isolates the allocation cost of a single doFlush call.
// Baseline expectation: 1 alloc per flush (make+copy of out slice).
// The callback writes to a package-level sink so it does not allocate.
func BenchmarkDoFlush(b *testing.B) {
	const chunkSize = 32 * 1024

	chunk := make([]byte, chunkSize)

	bt := &Batcher{
		buf:      make([]byte, 0, chunkSize),
		interval: time.Hour,
		onFlush:  storeFlushSink,
		done:     make(chan struct{}),
		flush:    make(chan struct{}, 1),
	}

	b.SetBytes(int64(chunkSize))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bt.buf = append(bt.buf[:0], chunk...)
		bt.doFlush()
	}
}

// flushSink is a package-level destination for benchmark onFlush callbacks
// so the callback does not allocate (no closure capture).
var flushSinkLen int

func storeFlushSink(data []byte) {
	flushSinkLen = len(data)
}

// BenchmarkBatcherBurstLatency measures the wall-clock time between Add and
// the onFlush callback firing, with the production timer interval (16 ms).
//
// Without a size threshold, this would average ~8 ms (half the timer
// interval). With the size threshold, a burst that exceeds the threshold
// flushes immediately and the latency drops to scheduling overhead.
func BenchmarkBatcherBurstLatency(b *testing.B) {
	// Burst chunk size deliberately larger than defaultFlushThreshold so a
	// single Add crosses the threshold.
	const burstChunkSize = 64 * 1024

	chunk := make([]byte, burstChunkSize)
	for i := range chunk {
		chunk[i] = 0x41
	}

	flushed := make(chan struct{}, 1024)
	batcher := newBatcher(16*time.Millisecond, func(_ []byte) {
		select {
		case flushed <- struct{}{}:
		default:
		}
	})
	defer batcher.Stop()

	b.SetBytes(int64(burstChunkSize))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Drain any stale signal from prior iteration.
		select {
		case <-flushed:
		default:
		}
		batcher.Add(chunk)
		<-flushed
	}
}

// BenchmarkBatcherSubThresholdLatency measures latency when each Add stays
// below the size threshold. Should remain ~8 ms (half the timer interval)
// — the size threshold must NOT shorten timer-driven flushes.
func BenchmarkBatcherSubThresholdLatency(b *testing.B) {
	// 1 KB is well under the 32 KB threshold.
	const smallChunkSize = 1024

	chunk := make([]byte, smallChunkSize)

	flushed := make(chan struct{}, 1024)
	batcher := newBatcher(16*time.Millisecond, func(_ []byte) {
		select {
		case flushed <- struct{}{}:
		default:
		}
	})
	defer batcher.Stop()

	b.SetBytes(int64(smallChunkSize))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		select {
		case <-flushed:
		default:
		}
		batcher.Add(chunk)
		<-flushed
	}
}

// BenchmarkBufferWriteRead measures the cost of a Write→Read cycle on the
// backpressure Buffer with the tracer disabled (production-typical).
func BenchmarkBufferWriteRead(b *testing.B) {
	const chunkSize = 32 * 1024

	chunk := make([]byte, chunkSize)
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "bench-sess")

	b.SetBytes(int64(chunkSize))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = buf.Write(chunk)
		_ = buf.Read()
	}
}
