package session

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
