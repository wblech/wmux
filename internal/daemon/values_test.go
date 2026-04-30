package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDataPayload(t *testing.T) {
	payload := EncodeDataPayload("my-session", []byte("hello world"))
	sid, data, err := DecodeDataPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, "my-session", sid)
	assert.Equal(t, []byte("hello world"), data)
}

func TestDecodeDataPayload_TooShort(t *testing.T) {
	_, _, err := DecodeDataPayload(nil)
	require.Error(t, err)

	_, _, err = DecodeDataPayload([]byte{5})
	require.Error(t, err)
}

func TestDecodeDataPayload_EmptyData(t *testing.T) {
	payload := EncodeDataPayload("s1", nil)
	sid, data, err := DecodeDataPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, "s1", sid)
	assert.Empty(t, data)
}

func TestEncodeInputPayload(t *testing.T) {
	payload := EncodeInputPayload("my-session", []byte("ls\n"))
	sid, data, err := DecodeInputPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, "my-session", sid)
	assert.Equal(t, []byte("ls\n"), data)
}

// TestAcquireDataPayload_BorrowedNotCopied documents and gates the pool
// contract for AcquireDataPayload: the returned slice is borrowed from
// dataPayloadPool and is only valid until ReleaseDataPayload(handle). After
// release, the same backing array is reused by the next Acquire call, so
// any retained reference will read mutated bytes.
//
// This test deliberately violates the contract (retains the slice across
// release+reacquire) and asserts that the bytes change — proving the pool
// is reusing the array. If a future refactor allocates fresh on every
// Acquire, the retained slice would still hold the first payload's bytes
// and this test fails, signaling that the zero-alloc broadcast property
// regressed.
//
// DO NOT use this code as a template — retaining the slice past Release IS
// the bug we are demonstrating. Production callers must finish broadcasting
// before Release.
func TestAcquireDataPayload_BorrowedNotCopied(t *testing.T) {
	first, h1 := AcquireDataPayload("sess-1", []byte("AAAAAAAA"))
	retained := first // intentional anti-pattern
	expectedFirst := append([]byte(nil), first...)
	assert.Equal(t, expectedFirst, retained)

	ReleaseDataPayload(h1)

	// Reacquire with same length — pool hands back the same backing array;
	// the append over the reset slice overwrites bytes that `retained` aliases.
	second, h2 := AcquireDataPayload("sess-2", []byte("BBBBBBBB"))
	defer ReleaseDataPayload(h2)

	// Sanity: second is the freshly encoded payload.
	sid, data, err := DecodeDataPayload(second)
	require.NoError(t, err)
	assert.Equal(t, "sess-2", sid)
	assert.Equal(t, []byte("BBBBBBBB"), data)

	// Borrow contract: retained now reflects the second Acquire's bytes,
	// NOT the original "sess-1"/"AAAAAAAA" payload it was acquired with.
	// If this fails, AcquireDataPayload is allocating fresh per call —
	// the zero-alloc broadcast hot path regressed.
	assert.NotEqual(t, expectedFirst, retained,
		"dataPayloadPool must reuse the backing array; if retained still "+
			"matches the first payload, AcquireDataPayload is allocating "+
			"fresh and the zero-alloc property regressed")
}
