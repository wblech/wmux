package debug

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvConfig_Disabled(t *testing.T) {
	cfg := ReadEnv()
	assert.False(t, cfg.Enabled)
	assert.Empty(t, cfg.Path)
	assert.Equal(t, LevelOff, cfg.Level)
}

func TestEnvConfig_WMUX_DEBUG(t *testing.T) {
	t.Setenv("WMUX_DEBUG", "1")
	cfg := ReadEnv()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, LevelChunk, cfg.Level)
	assert.NotEmpty(t, cfg.Path, "should have a default path")
}

func TestEnvConfig_WMUX_DEBUG_PATH(t *testing.T) {
	t.Setenv("WMUX_DEBUG_PATH", "/tmp/custom.log")
	cfg := ReadEnv()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "/tmp/custom.log", cfg.Path)
	assert.Equal(t, LevelChunk, cfg.Level)
}

func TestEnvConfig_WMUX_DEBUG_LEVEL(t *testing.T) {
	t.Setenv("WMUX_DEBUG", "1")
	t.Setenv("WMUX_DEBUG_LEVEL", "1")
	cfg := ReadEnv()
	assert.Equal(t, LevelLifecycle, cfg.Level)
}

func TestEnvConfig_WMUX_DEBUG_LEVEL_Clamp(t *testing.T) {
	t.Setenv("WMUX_DEBUG", "1")

	t.Setenv("WMUX_DEBUG_LEVEL", "99")
	cfg := ReadEnv()
	assert.Equal(t, LevelFull, cfg.Level)

	t.Setenv("WMUX_DEBUG_LEVEL", "-5")
	cfg = ReadEnv()
	assert.Equal(t, LevelOff, cfg.Level)
}

func TestEnvConfig_MaxSize(t *testing.T) {
	t.Setenv("WMUX_DEBUG", "1")
	t.Setenv("WMUX_DEBUG_MAX_SIZE_MB", "100")
	cfg := ReadEnv()
	assert.Equal(t, 100, cfg.MaxSizeMB)
}

func TestEnvConfig_MaxFiles(t *testing.T) {
	t.Setenv("WMUX_DEBUG", "1")
	t.Setenv("WMUX_DEBUG_MAX_FILES", "3")
	cfg := ReadEnv()
	assert.Equal(t, 3, cfg.MaxFiles)
}

func TestEnvConfig_DefaultPath_Platform(t *testing.T) {
	t.Setenv("WMUX_DEBUG", "1")
	cfg := ReadEnv()

	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, "Library", "Logs", "wmux", "wmux-debug.log")
		assert.Equal(t, expected, cfg.Path)
	} else {
		assert.Contains(t, cfg.Path, "wmux-debug.log")
	}
}

func TestClampLevel(t *testing.T) {
	assert.Equal(t, LevelOff, ClampLevel(-1))
	assert.Equal(t, LevelOff, ClampLevel(0))
	assert.Equal(t, LevelLifecycle, ClampLevel(1))
	assert.Equal(t, LevelChunk, ClampLevel(2))
	assert.Equal(t, LevelFull, ClampLevel(3))
	assert.Equal(t, LevelFull, ClampLevel(99))
}
