package session

import (
	"sync"
	"time"
)

// minBatchInterval is the smallest allowed flush interval.
const minBatchInterval = time.Millisecond

// defaultFlushThreshold is the buffer size at which Add will trigger an
// immediate flush instead of waiting for the next ticker. Sized to roughly
// match the PTY readChunkSize so a single hot read does not stall behind the
// 16 ms timer when a burst arrives.
const defaultFlushThreshold = 32 * 1024

// flushBufPool reuses []byte buffers handed to onFlush callbacks. The slice
// passed to onFlush is borrowed from this pool — its backing array MAY be
// recycled by a later flush, so retaining it past the callback is a silent
// data-corruption bug.
//
// Contract:
//
//	WRONG:  func(data []byte) { captured = data }       // alias may mutate
//	WRONG:  func(data []byte) { go process(data) }      // outlives callback
//	OK:     func(data []byte) { copy(dst, data) }       // copy is safe
//	OK:     func(data []byte) { buf.Write(data) }       // append copies
//
// Production callers (Buffer.Write via append) and tests already copy.
// Regression gate: BenchmarkBatcherAddFlush / BenchmarkDoFlush must report
// 0 allocs/op. If those regress to >0, the pool stopped working and
// retained-slice bugs in callers stop being silent (they now alias fresh
// allocations and look "fine") — fix the pool, do not relax the bench.
var flushBufPool = sync.Pool{
	New: func() any {
		s := make([]byte, 0, 64*1024)
		return &s
	},
}

// Batcher accumulates bytes and flushes them to a callback on a configurable interval.
// It runs an internal goroutine that drives timer-based flushes.
//
// In addition to the timer, Batcher signals an immediate flush when the
// pending buffer grows past flushThreshold. This bounds the worst-case
// latency on bursts to the threshold size, not the timer interval.
type Batcher struct {
	mu             sync.Mutex
	buf            []byte
	interval       time.Duration
	flushThreshold int
	onFlush        func([]byte)
	done           chan struct{}
	flush          chan struct{}
}

// newBatcher creates a Batcher that calls onFlush with accumulated data at the given interval.
// If interval is less than 1ms, it is clamped to 1ms.
// The internal flush goroutine starts immediately.
func newBatcher(interval time.Duration, onFlush func([]byte)) *Batcher {
	if interval < minBatchInterval {
		interval = minBatchInterval
	}

	b := &Batcher{
		mu:             sync.Mutex{},
		buf:            nil,
		interval:       interval,
		flushThreshold: defaultFlushThreshold,
		onFlush:        onFlush,
		done:           make(chan struct{}),
		flush:          make(chan struct{}, 1),
	}

	go b.loop()

	return b
}

// Add appends data to the current batch. If the buffer grows past
// flushThreshold, Add signals an immediate flush so bursts do not stall
// behind the next timer tick.
func (b *Batcher) Add(data []byte) {
	b.mu.Lock()
	b.buf = append(b.buf, data...)
	overThreshold := b.flushThreshold > 0 && len(b.buf) >= b.flushThreshold
	b.mu.Unlock()

	if overThreshold {
		select {
		case b.flush <- struct{}{}:
		default:
		}
	}
}

// FlushNow triggers an immediate flush outside of the normal interval.
// It returns as soon as the flush signal is sent; the actual flush happens
// asynchronously in the internal goroutine.
func (b *Batcher) FlushNow() {
	select {
	case b.flush <- struct{}{}:
	default:
	}
}

// Len returns the number of bytes currently waiting in the batch.
func (b *Batcher) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.buf)
}

// Stop shuts down the internal goroutine. Any pending data in the buffer is
// discarded. Calling Stop more than once is safe.
func (b *Batcher) Stop() {
	select {
	case <-b.done:
		// already stopped
	default:
		close(b.done)
	}
}

// loop is the internal flush goroutine.
func (b *Batcher) loop() {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-b.flush:
			b.doFlush()
		case <-ticker.C:
			b.doFlush()
		}
	}
}

// doFlush drains the buffer and calls onFlush if there is data.
//
// The slice passed to onFlush is borrowed from a pool and is only valid for
// the duration of the call. Callers that need to retain the data must copy
// it. The Buffer.Write call site copies via append, so this is safe in
// production.
func (b *Batcher) doFlush() {
	b.mu.Lock()

	if len(b.buf) == 0 {
		b.mu.Unlock()
		return
	}

	outPtr := flushBufPool.Get().(*[]byte)
	*outPtr = append((*outPtr)[:0], b.buf...)
	out := *outPtr
	b.buf = b.buf[:0]

	b.mu.Unlock()

	b.onFlush(out)

	*outPtr = out[:0]
	flushBufPool.Put(outPtr)
}
