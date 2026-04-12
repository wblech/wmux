// Package event provides an in-process event bus for wmux lifecycle events.
package event

import "errors"

// Type identifies the kind of event.
type Type int

// Phase 1 lifecycle event types.
const (
	_ Type = iota
	// SessionCreated is emitted when a new session is spawned.
	SessionCreated
	// SessionAttached is emitted when a client attaches to a session.
	SessionAttached
	// SessionDetached is emitted when a session has no attached clients.
	SessionDetached
	// SessionExited is emitted when a session's process exits.
	SessionExited
)

// String returns the dot-notation name of the event type.
func (t Type) String() string {
	switch t {
	case SessionCreated:
		return "session.created"
	case SessionAttached:
		return "session.attached"
	case SessionDetached:
		return "session.detached"
	case SessionExited:
		return "session.exited"
	default:
		return "unknown"
	}
}

// Event represents a single lifecycle event emitted by the daemon.
type Event struct {
	// Type identifies the event kind.
	Type Type `json:"type"`
	// SessionID is the session this event relates to.
	SessionID string `json:"session_id"`
	// Payload contains event-specific data as key-value pairs.
	Payload map[string]any `json:"payload,omitempty"`
}

// Sentinel errors for event operations.
var (
	// ErrBusClosed is returned when publishing to or subscribing on a closed bus.
	ErrBusClosed = errors.New("event: bus closed")
)
