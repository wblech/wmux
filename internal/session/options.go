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

// WithSpawnSemaphore limits the number of concurrent session spawns.
// A value of zero or less means no limit.
func WithSpawnSemaphore(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.spawnSem = make(chan struct{}, n)
		}
	}
}

// WithAddonManager sets the addon manager for creating addon-backed emulators.
// When set, sessions use AddonEmulator instead of NoneEmulator.
func WithAddonManager(mgr *AddonManager) Option {
	return func(s *Service) { s.addonManager = mgr }
}

// WithEmulatorFactory sets a factory for creating in-process emulators.
// When set, takes precedence over AddonManager for emulator creation.
func WithEmulatorFactory(f EmulatorFactory) Option {
	return func(s *Service) { s.emulatorFactory = f }
}
