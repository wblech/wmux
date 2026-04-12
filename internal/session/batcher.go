package session

import (
	"sync"
	"time"
)

// minBatchInterval is the smallest allowed flush interval.
const minBatchInterval = time.Millisecond

// Batcher accumulates bytes and flushes them to a callback on a configurable interval.
// It runs an internal goroutine that drives timer-based flushes.
type Batcher struct {
	mu       sync.Mutex
	buf      []byte
	interval time.Duration
	onFlush  func([]byte)
	done     chan struct{}
	flush    chan struct{}
}

// NewBatcher creates a Batcher that calls onFlush with accumulated data at the given interval.
// If interval is less than 1ms, it is clamped to 1ms.
// The internal flush goroutine starts immediately.
func NewBatcher(interval time.Duration, onFlush func([]byte)) *Batcher {
	if interval < minBatchInterval {
		interval = minBatchInterval
	}

	b := &Batcher{
		mu:       sync.Mutex{},
		buf:      nil,
		interval: interval,
		onFlush:  onFlush,
		done:     make(chan struct{}),
		flush:    make(chan struct{}, 1),
	}

	go b.loop()

	return b
}

// Add appends data to the current batch.
func (b *Batcher) Add(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buf = append(b.buf, data...)
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
func (b *Batcher) doFlush() {
	b.mu.Lock()

	if len(b.buf) == 0 {
		b.mu.Unlock()
		return
	}

	// Copy so the caller owns the slice independently.
	out := make([]byte, len(b.buf))
	copy(out, b.buf)
	b.buf = b.buf[:0]

	b.mu.Unlock()

	b.onFlush(out)
}
