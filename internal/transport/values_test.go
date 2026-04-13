package transport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAutomationMode(t *testing.T) {
	tests := []struct {
		input string
		want  AutomationMode
	}{
		{"open", ModeOpen},
		{"same-user", ModeSameUser},
		{"children", ModeChildren},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseAutomationMode(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseAutomationMode_Invalid(t *testing.T) {
	_, err := ParseAutomationMode("invalid")
	assert.ErrorIs(t, err, ErrInvalidAutomationMode)
}

func TestParseAutomationMode_Empty(t *testing.T) {
	_, err := ParseAutomationMode("")
	assert.ErrorIs(t, err, ErrInvalidAutomationMode)
}
