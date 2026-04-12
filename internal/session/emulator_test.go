package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoneEmulator_Process(t *testing.T) {
	e := NoneEmulator{}

	assert.NotPanics(t, func() { e.Process(nil) })
	assert.NotPanics(t, func() { e.Process([]byte{}) })
	assert.NotPanics(t, func() { e.Process([]byte("some terminal output\r\n")) })
	assert.NotPanics(t, func() { e.Process([]byte{0x1b, '[', '2', 'J'}) }) // ANSI escape
}

func TestNoneEmulator_Snapshot(t *testing.T) {
	e := NoneEmulator{}
	snap := e.Snapshot()

	assert.Nil(t, snap.Scrollback)
	assert.Nil(t, snap.Viewport)
}

func TestNoneEmulator_Resize(t *testing.T) {
	e := NoneEmulator{}

	assert.NotPanics(t, func() { e.Resize(80, 24) })
	assert.NotPanics(t, func() { e.Resize(0, 0) })
	assert.NotPanics(t, func() { e.Resize(-1, -1) })
}

func TestNoneEmulator_ImplementsInterface(_ *testing.T) {
	// Compile-time check: NoneEmulator must satisfy ScreenEmulator.
	var _ ScreenEmulator = NoneEmulator{}
}
