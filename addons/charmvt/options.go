package charmvt

// Option configures the charmvt backend.
type Option func(*config)

// Callbacks allows the integrator to observe terminal events.
type Callbacks struct {
	Bell             func(sessionID string)
	Title            func(sessionID string, title string)
	WorkingDirectory func(sessionID string, dir string)
	AltScreen        func(sessionID string, active bool)
}

// Logger is the interface for emulator debug output.
type Logger interface {
	Printf(format string, v ...any)
}

// SnapshotScrollbackMode controls which scrollback lines are included
// in the Replay stream produced by Snapshot().
type SnapshotScrollbackMode int

const (
	// SnapshotScrollbackAll includes the entire scrollback buffer.
	// This is the default and preserves backward compatibility.
	SnapshotScrollbackAll SnapshotScrollbackMode = iota

	// SnapshotScrollbackSinceLastClear includes only scrollback lines
	// added after the most recent ED2 (Erase Display 2) on the main
	// screen. Pre-ED2 content (e.g., shell output from before a TUI
	// started) is excluded from the Replay.
	SnapshotScrollbackSinceLastClear
)

type config struct {
	scrollback     int
	scrollbackMode SnapshotScrollbackMode
	callbacks      *Callbacks
	logger         Logger
}

func defaultConfig() *config {
	return &config{scrollback: 10000, scrollbackMode: SnapshotScrollbackAll, callbacks: nil, logger: nil}
}

// WithScrollbackSize sets the scrollback buffer size in lines. Default: 10000.
func WithScrollbackSize(lines int) Option {
	return func(c *config) { c.scrollback = lines }
}

// WithCallbacks sets event callbacks for all sessions created by this backend.
func WithCallbacks(cb Callbacks) Option {
	return func(c *config) { c.callbacks = &cb }
}

// WithLogger sets a debug logger for the emulator internals.
func WithLogger(l Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithSnapshotScrollbackMode sets the scrollback filtering mode for snapshots.
// Default: SnapshotScrollbackAll (include everything, backward-compatible).
func WithSnapshotScrollbackMode(mode SnapshotScrollbackMode) Option {
	return func(c *config) { c.scrollbackMode = mode }
}
