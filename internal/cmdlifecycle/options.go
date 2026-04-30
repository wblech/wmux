package cmdlifecycle

import "time"

// Option configures the Service at construction.
type Option func(*Service)

// WithMaxHistory overrides the per-session FIFO history cap.
// Default is defaultMaxHistory (500). Non-positive values are ignored.
func WithMaxHistory(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.maxHistory = n
		}
	}
}

// WithClock injects a deterministic clock for tests. Default is time.Now.
// Nil is ignored.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}
