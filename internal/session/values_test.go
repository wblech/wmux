package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSessionID_Valid(t *testing.T) {
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
}

func TestValidateSessionID_Invalid(t *testing.T) {
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
			assert.ErrorIs(t, ValidateSessionID(id), ErrInvalidSessionID)
		})
	}
}
