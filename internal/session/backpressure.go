package session

import (
	"sync"

	"github.com/wblech/wmux/internal/platform/debug"
)

// Buffer is a thread-safe byte buffer with high/low watermark backpressure.
// When the buffer size exceeds the high watermark, it enters a paused state.
// It resumes when the buffer drains below the low watermark.
type Buffer struct {
	mu            sync.Mutex
	data          []byte
	highWatermark int
	lowWatermark  int
	paused        bool
	tracer        *debug.Tracer
	sessionID     string
}

// newBuffer creates a new Buffer with the given high and low watermark thresholds.
// highWatermark is the size at which the buffer enters a paused state.
// lowWatermark is the size at which a paused buffer resumes.
func newBuffer(highWatermark, lowWatermark int, tracer *debug.Tracer, sessionID string) *Buffer {
	return &Buffer{
		mu:            sync.Mutex{},
		data:          nil,
		highWatermark: highWatermark,
		lowWatermark:  lowWatermark,
		paused:        false,
		tracer:        tracer,
		sessionID:     sessionID,
	}
}

// Write appends p to the buffer. If the total size exceeds the high watermark,
// the buffer enters a paused state. It always returns len(p), nil.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, p...)

	wasPaused := b.paused
	if len(b.data) >= b.highWatermark {
		b.paused = true
	}

	if b.tracer.Enabled() {
		b.tracer.Emit(debug.Event{
			SessionID:  b.sessionID,
			Stage:      debug.StageBufferAppend,
			Seq:        -1,
			ByteLen:    len(p),
			BufferSize: len(b.data),
			BufferHWM:  b.highWatermark,
		})

		if !wasPaused && b.paused {
			b.tracer.Emit(debug.Event{
				SessionID:  b.sessionID,
				Stage:      debug.StageBufferPause,
				Seq:        -1,
				BufferSize: len(b.data),
				BufferHWM:  b.highWatermark,
				Paused:     true,
			})
		}
	}

	return len(p), nil
}

// Read drains all data from the buffer and returns it.
// Returns nil if the buffer is empty.
// If the buffer was paused and the drain brings it below the low watermark,
// the paused state is cleared.
func (b *Buffer) Read() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) == 0 {
		return nil
	}

	out := b.data
	b.data = nil

	wasPaused := b.paused
	b.checkResume(0)

	if b.tracer.Enabled() {
		b.tracer.Emit(debug.Event{
			SessionID:  b.sessionID,
			Stage:      debug.StageBufferFlush,
			Seq:        -1,
			ByteLen:    len(out),
			BufferSize: 0,
		})

		if wasPaused && !b.paused {
			b.tracer.Emit(debug.Event{
				SessionID:  b.sessionID,
				Stage:      debug.StageBufferResume,
				Seq:        -1,
				BufferSize: 0,
				BufferLWM:  b.lowWatermark,
			})
		}
	}

	return out
}

// ReadN reads up to n bytes from the buffer and returns them.
// Returns nil if the buffer is empty.
// If the buffer was paused and the remaining data falls below the low watermark,
// the paused state is cleared.
func (b *Buffer) ReadN(n int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) == 0 {
		return nil
	}

	var out []byte
	if n >= len(b.data) {
		out = b.data
		b.data = nil
	} else {
		out = make([]byte, n)
		copy(out, b.data[:n])
		b.data = b.data[n:]
	}

	wasPaused := b.paused
	b.checkResume(len(b.data))

	if b.tracer.Enabled() {
		b.tracer.Emit(debug.Event{
			SessionID:  b.sessionID,
			Stage:      debug.StageBufferFlush,
			Seq:        -1,
			ByteLen:    len(out),
			BufferSize: len(b.data),
		})

		if wasPaused && !b.paused {
			b.tracer.Emit(debug.Event{
				SessionID:  b.sessionID,
				Stage:      debug.StageBufferResume,
				Seq:        -1,
				BufferSize: len(b.data),
				BufferLWM:  b.lowWatermark,
			})
		}
	}

	return out
}

// Paused reports whether the buffer is currently in a paused (backpressure) state.
func (b *Buffer) Paused() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.paused
}

// Len returns the current number of bytes held in the buffer.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.data)
}

// checkResume clears the paused flag if remaining is below the low watermark.
// Must be called with b.mu held.
func (b *Buffer) checkResume(remaining int) {
	if b.paused && remaining < b.lowWatermark {
		b.paused = false
	}
}
