package client

import "time"

// Option configures a client or daemon instance.
type Option func(*config)

// config holds resolved configuration for client and daemon construction.
type config struct {
	namespace         string
	baseDir           string
	socket            string
	tokenPath         string
	dataDir           string
	coldRestore       bool
	maxScrollbackSize int64
	autoStart         bool
	rpcTimeout        time.Duration
	emulatorFactory   EmulatorFactory
}

// newConfig creates a config with defaults and applies the given options.
func newConfig(opts ...Option) *config {
	cfg := &config{
		namespace:         "default",
		baseDir:           "",
		socket:            "",
		tokenPath:         "",
		dataDir:           "",
		coldRestore:       false,
		maxScrollbackSize: 0,
		autoStart:         true,
		rpcTimeout:        10 * time.Second,
		emulatorFactory:   nil,
	}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// WithNamespace sets the namespace for the client or daemon.
func WithNamespace(name string) Option {
	return func(c *config) { c.namespace = name }
}

// WithBaseDir sets the base directory for the client or daemon.
func WithBaseDir(path string) Option {
	return func(c *config) { c.baseDir = path }
}

// WithSocket sets the Unix socket path for IPC communication.
func WithSocket(path string) Option {
	return func(c *config) { c.socket = path }
}

// WithTokenPath sets the path to the authentication token file.
func WithTokenPath(path string) Option {
	return func(c *config) { c.tokenPath = path }
}

// WithDataDir sets the data directory for persistent storage.
func WithDataDir(path string) Option {
	return func(c *config) { c.dataDir = path }
}

// WithAutoStart controls whether the daemon is started automatically if not running.
func WithAutoStart(enabled bool) Option {
	return func(c *config) { c.autoStart = enabled }
}

// WithColdRestore controls whether sessions are restored from cold storage on startup.
func WithColdRestore(enabled bool) Option {
	return func(c *config) { c.coldRestore = enabled }
}

// WithMaxScrollbackSize sets the maximum scrollback buffer size in bytes.
func WithMaxScrollbackSize(n int64) Option {
	return func(c *config) { c.maxScrollbackSize = n }
}

// WithEmulatorFactory configures a custom emulator factory.
// This is the primary integration point for addon modules.
func WithEmulatorFactory(f EmulatorFactory) Option {
	return func(c *config) { c.emulatorFactory = f }
}

// WithRPCTimeout sets the maximum time to wait for a daemon response.
// If the daemon does not respond within this duration, the RPC returns
// ErrRequestTimeout. Default is 10 seconds.
func WithRPCTimeout(d time.Duration) Option {
	return func(c *config) { c.rpcTimeout = d }
}
