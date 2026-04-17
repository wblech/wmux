package session

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/platform/pty"
)

// slowEmulator simulates a ScreenEmulator whose Process() blocks for a
// configurable duration. Used to verify the readLoop doesn't stall when
// the emulator is slow.
type slowEmulator struct {
	mu       sync.Mutex
	delay    time.Duration
	chunks   [][]byte
	snapshot Snapshot
}

func (e *slowEmulator) Process(data []byte) {
	if e.delay > 0 {
		time.Sleep(e.delay)
	}
	e.mu.Lock()
	cp := make([]byte, len(data))
	copy(cp, data)
	e.chunks = append(e.chunks, cp)
	e.mu.Unlock()
}

func (e *slowEmulator) Snapshot() Snapshot { return e.snapshot }
func (e *slowEmulator) Resize(_, _ int)    {}

// panicEmulator panics on every Process() call.
type panicEmulator struct {
	mu     sync.Mutex
	panics int
}

func (e *panicEmulator) Process(_ []byte) {
	e.mu.Lock()
	e.panics++
	e.mu.Unlock()
	panic("emulator: unsupported sequence")
}

func (e *panicEmulator) Snapshot() Snapshot {
	return Snapshot{Replay: nil}
}
func (e *panicEmulator) Resize(_, _ int) {}

func (e *panicEmulator) panicCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.panics
}

// TestReadLoop_EmulatorSlowDoesNotBlockBroadcast verifies that when the
// emulator.Process() is slow, PTY data still reaches the batcher (and
// therefore broadcast) without delay.
//
// Regression test for: TUI apps (vim, Claude Code) hang because
// emulator.Process() blocks the readLoop, preventing PTY reads.
func TestReadLoop_EmulatorSlowDoesNotBlockBroadcast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	emulator := &slowEmulator{
		mu:       sync.Mutex{},
		delay:    500 * time.Millisecond,
		chunks:   nil,
		snapshot: Snapshot{Replay: nil},
	}

	svc := NewService(&pty.UnixSpawner{}, WithEmulatorFactory(fixedEmulatorFactory(emulator)))

	opts := defaultCreateOpts()
	opts.Shell = defaultShell()
	opts.Args = []string{"-c", "echo HELLO; echo WORLD; sleep 1"}
	opts.BatchInterval = 5 * time.Millisecond

	_, err := svc.Create("slow-emu-test", opts)
	require.NoError(t, err)

	// Wait for batcher to flush — should happen within ~30ms even though
	// the emulator takes 500ms per chunk.
	var output []byte
	require.Eventually(t, func() bool {
		data, _ := svc.ReadOutput("slow-emu-test")
		output = append(output, data...)
		return len(output) > 0 && contains(output, "HELLO")
	}, 2*time.Second, 10*time.Millisecond, "broadcast data should arrive quickly despite slow emulator")

	assert.Contains(t, string(output), "HELLO")
}

// TestReadLoop_EmulatorPanicDoesNotKillReadLoop verifies that if the
// emulator panics, the readLoop continues reading from the PTY and
// broadcasting data.
//
// Regression test for: TUI apps emit non-standard sequences (DCS, SGR
// variants) that cause the charmbracelet/x/vt emulator to panic,
// killing the readLoop goroutine and deadlocking the terminal.
func TestReadLoop_EmulatorPanicDoesNotKillReadLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	emulator := &panicEmulator{mu: sync.Mutex{}, panics: 0}

	svc := NewService(&pty.UnixSpawner{}, WithEmulatorFactory(fixedEmulatorFactory(emulator)))

	opts := defaultCreateOpts()
	opts.Shell = defaultShell()
	// Multiple echos to trigger multiple Process() calls — each one panics.
	opts.Args = []string{"-c", "echo LINE1; sleep 0.1; echo LINE2; sleep 0.1; echo LINE3; sleep 1"}
	opts.BatchInterval = 5 * time.Millisecond

	_, err := svc.Create("panic-emu-test", opts)
	require.NoError(t, err)

	// All 3 lines should eventually appear in broadcast despite panics.
	var output []byte
	require.Eventually(t, func() bool {
		data, _ := svc.ReadOutput("panic-emu-test")
		output = append(output, data...)
		return contains(output, "LINE1") && contains(output, "LINE2") && contains(output, "LINE3")
	}, 5*time.Second, 20*time.Millisecond, "all output lines should arrive despite emulator panics")

	assert.Positive(t, emulator.panicCount(), "emulator should have panicked at least once")
}

// fixedEmulatorFactory returns an EmulatorFactory that always returns the
// given emulator, ignoring the session ID and dimensions.
func fixedEmulatorFactory(em ScreenEmulator) EmulatorFactory {
	return emulatorFactoryFunc(func(_ string, _, _ int) ScreenEmulator { return em })
}

// emulatorFactoryFunc adapts a function to the EmulatorFactory interface.
type emulatorFactoryFunc func(id string, cols, rows int) ScreenEmulator

func (f emulatorFactoryFunc) Create(id string, cols, rows int) ScreenEmulator {
	return f(id, cols, rows)
}

func (f emulatorFactoryFunc) Close() {}

func contains(data []byte, substr string) bool {
	return len(data) > 0 && len(substr) > 0 &&
		string(data) != "" &&
		containsBytes(data, []byte(substr))
}

func containsBytes(data, sub []byte) bool {
	for i := 0; i <= len(data)-len(sub); i++ {
		if string(data[i:i+len(sub)]) == string(sub) {
			return true
		}
	}
	return false
}
