package transport

// Option is a functional option for configuring a Server.
type Option func(*Server)

// WithAutomationMode sets the automation mode that governs which processes
// are permitted to connect to the server.
func WithAutomationMode(mode AutomationMode) Option {
	return func(s *Server) {
		s.mode = mode
	}
}
