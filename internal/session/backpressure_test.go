package session

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuffer_WriteAndRead(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")
	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	got := buf.Read()
	assert.Equal(t, []byte("hello"), got)
}

func TestBuffer_ReadDrains(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")
	_, err := buf.Write([]byte("data"))
	require.NoError(t, err)

	first := buf.Read()
	assert.Equal(t, []byte("data"), first)

	second := buf.Read()
	assert.Nil(t, second)
}

func TestBuffer_HighWatermarkPause(t *testing.T) {
	buf := newBuffer(10, 5, nil, "")
	assert.False(t, buf.Paused())

	_, err := buf.Write([]byte("0123456789")) // exactly 10 bytes == high watermark
	require.NoError(t, err)
	assert.True(t, buf.Paused())
}

func TestBuffer_LowWatermarkResume(t *testing.T) {
	buf := newBuffer(10, 5, nil, "")
	_, err := buf.Write([]byte("0123456789")) // trigger pause
	require.NoError(t, err)
	assert.True(t, buf.Paused())

	_ = buf.Read() // drain all
	assert.False(t, buf.Paused())
}

func TestBuffer_PartialDrainStaysPaused(t *testing.T) {
	buf := newBuffer(10, 5, nil, "")
	_, err := buf.Write([]byte("0123456789")) // 10 bytes -> paused
	require.NoError(t, err)
	assert.True(t, buf.Paused())

	// Read only 4 bytes; 6 remain, which is still >= lowWatermark (5)
	_ = buf.ReadN(4)
	assert.True(t, buf.Paused())
}

func TestBuffer_DrainBelowLowResumes(t *testing.T) {
	buf := newBuffer(10, 5, nil, "")
	_, err := buf.Write([]byte("0123456789")) // 10 bytes -> paused
	require.NoError(t, err)
	assert.True(t, buf.Paused())

	// Read 7 bytes; 3 remain, which is below lowWatermark (5) -> should resume
	_ = buf.ReadN(7)
	assert.False(t, buf.Paused())
}

func TestBuffer_MultipleWrites(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")
	_, err := buf.Write([]byte("foo"))
	require.NoError(t, err)
	_, err = buf.Write([]byte("bar"))
	require.NoError(t, err)
	_, err = buf.Write([]byte("baz"))
	require.NoError(t, err)

	got := buf.Read()
	assert.Equal(t, []byte("foobarbaz"), got)
}

func TestBuffer_ReadN_ExactLength(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")
	_, err := buf.Write([]byte("hello"))
	require.NoError(t, err)

	// n == len(data): should return all bytes and drain the buffer.
	out := buf.ReadN(5)
	assert.Equal(t, []byte("hello"), out)
	assert.Equal(t, 0, buf.Len())
}

func TestBuffer_ReadN_GreaterThanLength(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")
	_, err := buf.Write([]byte("hi"))
	require.NoError(t, err)

	// n > len(data): should return all available bytes.
	out := buf.ReadN(100)
	assert.Equal(t, []byte("hi"), out)
	assert.Equal(t, 0, buf.Len())
}

func TestBuffer_ReadN_Empty(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")

	// ReadN on empty buffer returns nil.
	out := buf.ReadN(10)
	assert.Nil(t, out)
}

func TestBuffer_Len(t *testing.T) {
	buf := newBuffer(100, 50, nil, "")
	assert.Equal(t, 0, buf.Len())

	_, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, buf.Len())

	_ = buf.ReadN(3)
	assert.Equal(t, 2, buf.Len())

	_ = buf.Read()
	assert.Equal(t, 0, buf.Len())
}

// TestBuffer_2CycleSwap_DataCorrectness verifies that data is returned correctly
// across many Write→Read cycles, including after the 2-cycle swap starts
// recycling backing arrays.
func TestBuffer_2CycleSwap_DataCorrectness(t *testing.T) {
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "test-2cycle")

	for i := 0; i < 20; i++ {
		payload := bytes.Repeat([]byte{byte(i)}, 1024)
		_, err := buf.Write(payload)
		require.NoError(t, err)

		got := buf.Read()
		assert.Equal(t, payload, got, "iteration %d: data mismatch", i)
	}
}

// TestBuffer_2CycleSwap_ZeroAllocsInSteadyState is the regression guard for
// the 2-cycle backing-array swap. After 2 warmup cycles (which allocate), all
// subsequent Write→Read pairs must produce 0 heap allocations.
func TestBuffer_2CycleSwap_ZeroAllocsInSteadyState(t *testing.T) {
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "test-allocs")
	chunk := make([]byte, 32*1024)
	for i := range chunk {
		chunk[i] = 0x41
	}

	// 2 warmup cycles to prime inFlight and spare.
	for range 2 {
		_, _ = buf.Write(chunk)
		_ = buf.Read()
	}

	allocs := testing.AllocsPerRun(50, func() {
		_, _ = buf.Write(chunk)
		_ = buf.Read()
	})

	if allocs > 0 {
		t.Errorf("expected 0 allocs/op in steady state after warmup, got %.1f", allocs)
	}
}

// TestBuffer_MultipleWritesThenRead_CycleChain confirms that multiple Writes
// accumulate correctly into a single backing array before each Read.
func TestBuffer_MultipleWritesThenRead_CycleChain(t *testing.T) {
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "test-multi")

	for range 5 {
		_, _ = buf.Write([]byte("foo"))
		_, _ = buf.Write([]byte("bar"))
		got := buf.Read()
		assert.Equal(t, []byte("foobar"), got)
	}
}

// TestBuffer_ReadNFullDrain_AdvancesChain ensures that a ReadN call that drains
// the full buffer (n >= len) advances the 2-cycle chain exactly like Read.
func TestBuffer_ReadNFullDrain_AdvancesChain(t *testing.T) {
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "test-readn-full")
	chunk := make([]byte, 32*1024)
	for i := range chunk {
		chunk[i] = 0x42
	}

	for range 2 {
		_, _ = buf.Write(chunk)
		_ = buf.ReadN(len(chunk)) // full drain via ReadN
	}

	allocs := testing.AllocsPerRun(50, func() {
		_, _ = buf.Write(chunk)
		_ = buf.ReadN(len(chunk))
	})

	if allocs > 0 {
		t.Errorf("ReadN full-drain: expected 0 allocs in steady state, got %.1f", allocs)
	}
}

// TestBuffer_PartialReadN_DoesNotAdvanceChain ensures that a partial ReadN
// (n < len) does NOT advance the 2-cycle chain (backing array still in use).
func TestBuffer_PartialReadN_DoesNotAdvanceChain(t *testing.T) {
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "test-readn-partial")

	_, _ = buf.Write([]byte("hello world"))
	out := buf.ReadN(5) // partial drain
	assert.Equal(t, []byte("hello"), out)

	// Remaining data still accessible.
	rest := buf.Read()
	assert.Equal(t, []byte(" world"), rest)
}

// TestBuffer_EmptyReadDoesNotCorruptChain verifies that calling Read on an
// empty buffer (returns nil) leaves inFlight and spare unchanged.
func TestBuffer_EmptyReadDoesNotCorruptChain(t *testing.T) {
	buf := newBuffer(4*1024*1024, 2*1024*1024, nil, "test-empty-read")
	chunk := make([]byte, 64)
	for i := range chunk {
		chunk[i] = 0x43
	}

	// Write and read once to set inFlight.
	_, _ = buf.Write(chunk)
	first := buf.Read()
	assert.Equal(t, chunk, first)

	// Empty read — must return nil and not corrupt the chain.
	assert.Nil(t, buf.Read())
	assert.Nil(t, buf.Read())

	// Next write+read must still work correctly.
	_, _ = buf.Write(chunk)
	second := buf.Read()
	assert.Equal(t, chunk, second)
}
