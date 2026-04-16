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

type config struct {
	scrollback int
	callbacks  *Callbacks
	logger     Logger
}

func defaultConfig() *config {
	return &config{scrollback: 10000}
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
