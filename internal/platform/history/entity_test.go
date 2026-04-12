package history

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSize_Zero(t *testing.T) {
	n, err := ParseSize("0")
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestParseSize_Bytes(t *testing.T) {
	n, err := ParseSize("1024")
	require.NoError(t, err)
	assert.Equal(t, int64(1024), n)
}

func TestParseSize_KB(t *testing.T) {
	n, err := ParseSize("512KB")
	require.NoError(t, err)
	assert.Equal(t, int64(512*1024), n)
}

func TestParseSize_MB(t *testing.T) {
	n, err := ParseSize("1MB")
	require.NoError(t, err)
	assert.Equal(t, int64(1*1024*1024), n)
}

func TestParseSize_GB(t *testing.T) {
	n, err := ParseSize("5GB")
	require.NoError(t, err)
	assert.Equal(t, int64(5*1024*1024*1024), n)
}

func TestParseSize_LowerCase(t *testing.T) {
	n, err := ParseSize("512kb")
	require.NoError(t, err)
	assert.Equal(t, int64(512*1024), n)
}

func TestParseSize_Empty(t *testing.T) {
	_, err := ParseSize("")
	assert.ErrorIs(t, err, ErrInvalidSize)
}

func TestParseSize_InvalidUnit(t *testing.T) {
	_, err := ParseSize("10XB")
	assert.ErrorIs(t, err, ErrInvalidSize)
}

func TestParseSize_Negative(t *testing.T) {
	_, err := ParseSize("-1")
	assert.ErrorIs(t, err, ErrInvalidSize)
}

func TestParseSize_NonNumeric(t *testing.T) {
	_, err := ParseSize("abc")
	assert.ErrorIs(t, err, ErrInvalidSize)
}
