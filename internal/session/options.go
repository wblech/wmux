package session

import "github.com/wblech/wmux/internal/platform/debug"

// Option is a functional option for configuring a Service.
type Option func(*Service)

// WithMaxSessions sets the maximum number of concurrent sessions the Service will allow.
// A value of zero or less means no limit.
func WithMaxSessions(n int) Option {
	return func(s *Service) { s.maxSessions = n }
}

// WithOnExit registers a callback invoked when a session's process exits.
// The callback receives the session ID and exit code.
func WithOnExit(fn func(id string, exitCode int)) Option {
	return func(s *Service) { s.onExit = fn }
}

// WithSpawnSemaphore limits the number of concurrent session spawns.
// A value of zero or less means no limit.
func WithSpawnSemaphore(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.spawnSem = make(chan struct{}, n)
		}
	}
}

// WithEmulatorFactory sets a factory for creating in-process emulators.
// When set, sessions use the factory emulator instead of NoneEmulator.
func WithEmulatorFactory(f EmulatorFactory) Option {
	return func(s *Service) { s.emulatorFactory = f }
}

// WithTracer sets the debug tracer for instrumentation.
func WithTracer(t *debug.Tracer) Option {
	return func(s *Service) { s.tracer = t }
}
