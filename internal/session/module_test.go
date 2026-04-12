package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestModule_IsValidFxOptions(t *testing.T) {
	// Module must be a valid fx.Options value (non-nil).
	assert.NotNil(t, Module)
	assert.IsType(t, fx.Options(), Module)
}
