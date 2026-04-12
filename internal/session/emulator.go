package session

// NoneEmulator is a no-op implementation of ScreenEmulator.
// It discards all input and returns empty snapshots.
// Use it when terminal output capture is not required.
type NoneEmulator struct{}

// Process discards the provided data without processing it.
func (NoneEmulator) Process(_ []byte) {}

// Snapshot returns an empty Snapshot with nil scrollback and viewport.
func (NoneEmulator) Snapshot() Snapshot {
	return Snapshot{
		Scrollback: nil,
		Viewport:   nil,
	}
}

// Resize accepts new terminal dimensions but performs no action.
func (NoneEmulator) Resize(_ int, _ int) {}
