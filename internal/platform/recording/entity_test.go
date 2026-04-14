package recording

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrRecordingClosed(t *testing.T) {
	assert.EqualError(t, ErrRecordingClosed, "recording: writer closed")
}

func TestErrSizeLimitReached(t *testing.T) {
	assert.EqualError(t, ErrSizeLimitReached, "recording: size limit reached")
}
