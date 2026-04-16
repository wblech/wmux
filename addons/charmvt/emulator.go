package charmvt

import (
	"strings"

	"github.com/charmbracelet/x/vt"
	"github.com/wblech/wmux/pkg/client"
)

// emulator wraps charmbracelet/x/vt as a client.ScreenEmulator.
type emulator struct {
	term *vt.SafeEmulator
	cols int
}

func newEmulator(sessionID string, cols, rows int, cfg *config) *emulator {
	term := vt.NewSafeEmulator(cols, rows)

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

	return &emulator{term: term, cols: cols}
}

// Process writes terminal data to the emulator.
func (e *emulator) Process(data []byte) {
	_, _ = e.term.Write(data)
}

// Snapshot returns the current terminal screen state.
// Viewport uses \r\n line endings with trailing empty rows stripped.
// Scrollback uses \r\n line endings.
func (e *emulator) Snapshot() client.Snapshot {
	viewport := e.term.Render()
	viewport = trimTrailingEmptyRows(viewport)
	viewport = toTerminalLineEndings(viewport)

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
	e.term.SetScrollbackSize(lines)
}

// Resize updates the terminal dimensions.
func (e *emulator) Resize(cols, rows int) {
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
