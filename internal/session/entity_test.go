package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateAlive, "alive"},
		{StateDetached, "detached"},
		{StateExited, "exited"},
		{StateRemoved, "removed"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestState_IsTerminal(t *testing.T) {
	assert.False(t, StateAlive.IsTerminal())
	assert.False(t, StateDetached.IsTerminal())
	assert.True(t, StateExited.IsTerminal())
	assert.True(t, StateRemoved.IsTerminal())
}

func TestSessionID_Validation(t *testing.T) {
	t.Run("valid IDs", func(t *testing.T) {
		valid := []string{
			"session-1",
			"my-project/session-1",
			"abc",
			"ABC_123",
			"a/b/c",
			"under_score",
		}

		for _, id := range valid {
			t.Run(id, func(t *testing.T) {
				require.NoError(t, ValidateSessionID(id))
			})
		}
	})

	t.Run("invalid IDs", func(t *testing.T) {
		invalid := []string{
			"",
			"has spaces",
			"../traversal",
			"path/../traversal",
			"dot..dot",
			"with.dot",
			"with@special",
			"with#hash",
		}

		for _, id := range invalid {
			t.Run(id, func(t *testing.T) {
				require.ErrorIs(t, ValidateSessionID(id), ErrInvalidSessionID)
			})
		}
	})
}
