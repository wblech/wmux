// Package recording provides asciinema v2 format recording for terminal sessions.
package recording

import "errors"

// Sentinel errors for recording operations.
var (
	// ErrRecordingClosed is returned when writing to a closed recorder.
	ErrRecordingClosed = errors.New("recording: writer closed")
	// ErrSizeLimitReached is returned when the recording file size limit is exceeded.
	ErrSizeLimitReached = errors.New("recording: size limit reached")
)
