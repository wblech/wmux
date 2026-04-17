package charmvt

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/x/vt"
	"github.com/wblech/wmux/pkg/client"
)

// emulator wraps charmbracelet/x/vt as a client.ScreenEmulator.
//
// The vt.Emulator uses an internal io.Pipe for terminal responses (DA1, DA2,
// DSR, CPR). Without a reader, Write() deadlocks because io.Pipe is
// synchronous. A background goroutine drains the pipe to prevent this.
// See: decisions/0025-drain-emulator-response-pipe.md
type emulator struct {
	mu   sync.Mutex
	term *vt.Emulator
	cols int
}

func newEmulator(sessionID string, cols, rows int, cfg *config) *emulator {
	term := vt.NewEmulator(cols, rows)

	term.SetScrollbackSize(cfg.scrollback)

	if cfg.logger != nil {
		term.SetLogger(cfg.logger)
	}

	if cfg.callbacks != nil {
		cb := vt.Callbacks{}
		if cfg.callbacks.Bell != nil {
			fn := cfg.callbacks.Bell
			cb.Bell = func() { fn(sessionID) }
		}
		if cfg.callbacks.Title != nil {
			fn := cfg.callbacks.Title
			cb.Title = func(title string) { fn(sessionID, title) }
		}
		if cfg.callbacks.WorkingDirectory != nil {
			fn := cfg.callbacks.WorkingDirectory
			cb.WorkingDirectory = func(dir string) { fn(sessionID, dir) }
		}
		if cfg.callbacks.AltScreen != nil {
			fn := cfg.callbacks.AltScreen
			cb.AltScreen = func(active bool) { fn(sessionID, active) }
		}
		term.SetCallbacks(cb)
	}

	// Drain the emulator's response pipe to prevent Write() from blocking.
	// The vt emulator writes DA1/DA2/DSR/CPR responses to an internal
	// io.Pipe. Without a reader, Write() deadlocks. These responses are
	// safely discarded — the real terminal (xterm.js) handles them.
	// The goroutine exits when term.Close() is called (pipe returns EOF).
	go drainResponsePipe(term)

	return &emulator{mu: sync.Mutex{}, term: term, cols: cols}
}

// drainResponsePipe reads and discards all data from the emulator's response
// pipe. This prevents vt.Write() from blocking when the emulator generates
// terminal responses (DA1, DA2, DSR, CPR). Exits when the pipe is closed.
func drainResponsePipe(term *vt.Emulator) {
	buf := make([]byte, 256)
	for {
		if _, err := term.Read(buf); err != nil {
			return
		}
	}
}

// Process writes terminal data to the emulator.
func (e *emulator) Process(data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = e.term.Write(data)
}

// Snapshot returns the current terminal screen state.
// Viewport uses \r\n line endings with trailing empty rows stripped.
// Scrollback uses \r\n line endings.
func (e *emulator) Snapshot() client.Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()

	viewport := e.term.Render()
	viewport = trimTrailingEmptyRows(viewport)
	viewport = toTerminalLineEndings(viewport)

	// Append a CUP (Cursor Position) escape to restore the cursor where
	// the emulator has it. Without this, xterm.js leaves the cursor at
	// the end of the last written text after a warm reconnect snapshot,
	// causing visual desync between cursor and prompt.
	pos := e.term.CursorPosition()
	viewport += fmt.Sprintf("\x1b[%d;%dH", pos.Y+1, pos.X+1)

	scrollback := renderScrollback(e.term, e.cols)

	return client.Snapshot{
		Viewport:   []byte(viewport),
		Scrollback: scrollback,
	}
}

// SetScrollbackSize changes the scrollback buffer size at runtime.
// Implements session.ScrollbackConfigurable.
// Increasing preserves existing data. Decreasing discards oldest lines.
func (e *emulator) SetScrollbackSize(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.term.SetScrollbackSize(lines)
}

// Close shuts down the emulator and stops the drain goroutine.
// Implements io.Closer so the session layer can clean up via type assertion.
func (e *emulator) Close() error {
	return e.term.Close()
}

// Resize updates the terminal dimensions.
func (e *emulator) Resize(cols, rows int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cols = cols
	e.term.Resize(cols, rows)
}

// trimTrailingEmptyRows removes trailing newlines that represent empty grid rows
// from the viewport render output.
func trimTrailingEmptyRows(s string) string {
	return strings.TrimRight(s, "\n")
}

// toTerminalLineEndings converts \n to \r\n for terminal frontend consumption.
// Terminal emulators require \r\n: \n alone moves the cursor down without
// returning to column 0.
func toTerminalLineEndings(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}
