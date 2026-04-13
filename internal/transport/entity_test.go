package transport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelType_String(t *testing.T) {
	assert.Equal(t, "control", ChannelControl.String())
	assert.Equal(t, "stream", ChannelStream.String())
	assert.Equal(t, "unknown", ChannelType(99).String())
}

func TestAutomationMode_String(t *testing.T) {
	assert.Equal(t, "open", ModeOpen.String())
	assert.Equal(t, "same-user", ModeSameUser.String())
	assert.Equal(t, "children", ModeChildren.String())
	assert.Equal(t, "unknown", AutomationMode(99).String())
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors have distinct messages.
	assert.NotEqual(t, ErrAuthFailed.Error(), ErrClientNotFound.Error())
	assert.NotEqual(t, ErrInvalidChannel.Error(), ErrDuplicateChannel.Error())
	assert.NotEqual(t, ErrServerClosed.Error(), ErrAuthFailed.Error())
}
