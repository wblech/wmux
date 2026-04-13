package daemon

// Option is a functional option for configuring a Daemon.
type Option func(*Daemon)

// WithVersion sets the version string reported in the PID file.
func WithVersion(v string) Option {
	return func(d *Daemon) {
		d.version = v
	}
}

// WithPIDFilePath sets the path at which the PID file is written on startup.
func WithPIDFilePath(path string) Option {
	return func(d *Daemon) {
		d.pidFilePath = path
	}
}

// WithDataDir sets the data directory used by the daemon.
func WithDataDir(dir string) Option {
	return func(d *Daemon) {
		d.dataDir = dir
	}
}

// WithEventBus sets the event bus used by the daemon to emit lifecycle events.
func WithEventBus(bus EventBus) Option {
	return func(d *Daemon) {
		d.eventBus = bus
	}
}

// WithColdRestore enables or disables cold restore (history persistence to disk).
// When enabled and a data directory is set, the daemon writes session metadata
// and scrollback to disk so sessions can be restored after a daemon restart.
func WithColdRestore(enabled bool) Option {
	return func(d *Daemon) {
		d.coldRestore = enabled
	}
}

// WithMaxScrollbackSize sets the maximum scrollback file size in bytes for cold restore.
// A value of 0 means unlimited.
func WithMaxScrollbackSize(n int64) Option {
	return func(d *Daemon) {
		d.maxScrollbackSize = n
	}
}
