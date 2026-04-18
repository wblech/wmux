package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracerConfig_Defaults(t *testing.T) {
	cfg := newTracerConfig()
	assert.Equal(t, 50, cfg.maxSizeMB)
	assert.Equal(t, 7, cfg.maxFiles)
}

func TestTracerConfig_WithMaxSize(t *testing.T) {
	cfg := newTracerConfig(WithMaxSize(100))
	assert.Equal(t, 100, cfg.maxSizeMB)
}

func TestTracerConfig_WithMaxFiles(t *testing.T) {
	cfg := newTracerConfig(WithMaxFiles(3))
	assert.Equal(t, 3, cfg.maxFiles)
}
