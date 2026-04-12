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
