package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSentinelErrors(t *testing.T) {
	assert.NotEqual(t, ErrDaemonRunning.Error(), ErrDaemonNotRunning.Error())
	assert.NotEqual(t, ErrAlreadyAttached.Error(), ErrNotAttached.Error())
	assert.NotEqual(t, ErrSessionNotSpecified.Error(), ErrAlreadyAttached.Error())
}
