package client

// ScreenEmulator processes terminal data and provides snapshots.
// Addon modules implement this interface to provide custom emulator backends.
type ScreenEmulator interface {
	// Process handles incoming terminal data bytes.
	Process(data []byte)
	// Snapshot returns the current terminal screen state.
	Snapshot() Snapshot
	// Resize updates the terminal dimensions.
	Resize(cols, rows int)
}

// EmulatorFactory creates ScreenEmulator instances for sessions.
// Addon modules implement this to provide custom emulator backends.
type EmulatorFactory interface {
	// Create returns a new ScreenEmulator for the given session.
	Create(sessionID string, cols, rows int) ScreenEmulator
	// Close shuts down the factory and releases resources.
	Close()
}
