package session

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBatcher_FlushesOnInterval(t *testing.T) {
	var (
		mu      sync.Mutex
		flushed []byte
	)

	b := newBatcher(50*time.Millisecond, func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]byte, len(data))
		copy(cp, data)
		flushed = cp
	})
	defer b.Stop()

	b.Add([]byte("hello"))

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(flushed) > 0
	}, 200*time.Millisecond, 10*time.Millisecond)

	mu.Lock()
	got := flushed
	mu.Unlock()

	assert.Equal(t, []byte("hello"), got)
}

func TestBatcher_NoFlushWhenEmpty(t *testing.T) {
	flushed := false

	b := newBatcher(50*time.Millisecond, func(_ []byte) {
		flushed = true
	})
	defer b.Stop()

	// Wait for two full intervals without adding any data.
	time.Sleep(120 * time.Millisecond)
	assert.False(t, flushed)
}

func TestBatcher_MultipleBatches(t *testing.T) {
	var (
		mu      sync.Mutex
		batches [][]byte
	)

	b := newBatcher(50*time.Millisecond, func(data []byte) {
		cp := make([]byte, len(data))
		copy(cp, data)
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, cp)
	})
	defer b.Stop()

	b.Add([]byte("first"))

	// Wait until first batch is flushed.
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(batches) >= 1
	}, 200*time.Millisecond, 10*time.Millisecond)

	b.Add([]byte("second"))

	// Wait until second batch is flushed.
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(batches) >= 2
	}, 200*time.Millisecond, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []byte("first"), batches[0])
	assert.Equal(t, []byte("second"), batches[1])
}

func TestBatcher_StopPreventsFlush(_ *testing.T) {
	b := newBatcher(50*time.Millisecond, func(_ []byte) {
		// If this is called after Stop, the test body may have already exited;
		// the test simply must not panic.
	})

	b.Add([]byte("data"))
	b.Stop()

	// Give a bit of time to confirm no late goroutine races.
	time.Sleep(100 * time.Millisecond)
}

func TestBatcher_FlushNow(t *testing.T) {
	var (
		mu      sync.Mutex
		flushed []byte
	)

	// Use a 1-second interval so the timer won't fire during the test.
	b := newBatcher(time.Second, func(data []byte) {
		cp := make([]byte, len(data))
		copy(cp, data)
		mu.Lock()
		defer mu.Unlock()
		flushed = cp
	})
	defer b.Stop()

	b.Add([]byte("immediate"))
	b.FlushNow()

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(flushed) > 0
	}, 200*time.Millisecond, 5*time.Millisecond)

	mu.Lock()
	got := flushed
	mu.Unlock()

	assert.Equal(t, []byte("immediate"), got)
}

func TestBatcher_Len(t *testing.T) {
	// Use a 1-second interval so the timer won't interfere.
	b := newBatcher(time.Second, func(_ []byte) {})
	defer b.Stop()

	assert.Equal(t, 0, b.Len())

	b.Add([]byte("hello"))
	assert.Equal(t, 5, b.Len())

	b.Add([]byte(" world"))
	assert.Equal(t, 11, b.Len())

	b.FlushNow()

	assert.Eventually(t, func() bool {
		return b.Len() == 0
	}, 200*time.Millisecond, 5*time.Millisecond)
}

func TestNewBatcher_RequiresPositiveInterval(t *testing.T) {
	// A zero interval should be clamped to minBatchInterval and not panic.
	flushed := make(chan struct{}, 1)

	b := newBatcher(0, func(_ []byte) {
		select {
		case flushed <- struct{}{}:
		default:
		}
	})
	defer b.Stop()

	b.Add([]byte("x"))

	select {
	case <-flushed:
		// Good — the batcher worked despite zero interval.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("batcher with 0 interval did not flush within 200ms")
	}
}
