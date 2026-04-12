package session

// Option is a functional option for configuring a Service.
type Option func(*Service)

// WithMaxSessions sets the maximum number of concurrent sessions the Service will allow.
// A value of zero or less means no limit.
func WithMaxSessions(n int) Option {
	return func(s *Service) { s.maxSessions = n }
}
