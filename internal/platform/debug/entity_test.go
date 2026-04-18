package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevel_Constants(t *testing.T) {
	assert.Equal(t, Level(0), LevelOff)
	assert.Equal(t, Level(1), LevelLifecycle)
	assert.Equal(t, Level(2), LevelChunk)
	assert.Equal(t, Level(3), LevelFull)
}

func TestStage_Constants(t *testing.T) {
	stages := []Stage{
		StagePtyRead, StageBufferAppend, StageBufferFlush,
		StageBufferPause, StageBufferResume,
		StageEmulatorIn, StageEmulatorOut, StageEmulatorDrop,
		StageFrameSend,
		StageSnapshotStart, StageSnapshotDone,
		StageResize, StageAttach, StageDetach,
		StageSessionCreate, StageSessionClose,
	}
	assert.Len(t, stages, 16, "spec requires exactly 16 stages")

	// No duplicate values.
	seen := make(map[Stage]bool)
	for _, s := range stages {
		assert.False(t, seen[s], "duplicate stage: %s", s)
		seen[s] = true
	}
}

func TestEvent_ZeroValue(t *testing.T) {
	var ev Event
	assert.Zero(t, ev.Time)
	assert.Empty(t, ev.SessionID)
	assert.Equal(t, Stage(""), ev.Stage)
	assert.Equal(t, int64(0), ev.Seq)
	assert.Equal(t, 0, ev.ByteLen)
}
