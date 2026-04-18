package debug

// TracerOption configures a Tracer.
type TracerOption func(*tracerConfig)

type tracerConfig struct {
	maxSizeMB int
	maxFiles  int
}

func newTracerConfig(opts ...TracerOption) *tracerConfig {
	cfg := &tracerConfig{
		maxSizeMB: 50,
		maxFiles:  7,
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// WithMaxSize sets the maximum size in megabytes before log rotation.
// Default: 50 MB.
func WithMaxSize(mb int) TracerOption {
	return func(c *tracerConfig) { c.maxSizeMB = mb }
}

// WithMaxFiles sets the maximum number of rotated log files to retain.
// Default: 7.
func WithMaxFiles(n int) TracerOption {
	return func(c *tracerConfig) { c.maxFiles = n }
}
