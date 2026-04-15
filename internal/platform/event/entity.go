// Package event provides an in-process event bus for wmux lifecycle events.
package event

import (
	"errors"
	"fmt"
)

// Type identifies the kind of event.
type Type int

// Lifecycle event types.
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
	// SessionIdle is emitted when a session has no activity for a configured period.
	SessionIdle
	// SessionKilled is emitted when a session is explicitly killed.
	SessionKilled
	// Resize is emitted when a session's terminal dimensions change.
	Resize
	// CwdChanged is emitted when a session's working directory changes (via OSC 7).
	CwdChanged
	// Notification is emitted when an OSC 9/99/777 notification is received.
	Notification
	// OutputFlood is emitted when a session's output rate exceeds a threshold.
	OutputFlood
	// RecordingLimitReached is emitted when a recording file size limit is exceeded.
	RecordingLimitReached
	// ShellReady is emitted when a session's shell signals readiness via OSC marker.
	ShellReady
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
	case SessionIdle:
		return "session.idle"
	case SessionKilled:
		return "session.killed"
	case Resize:
		return "resize"
	case CwdChanged:
		return "cwd.changed"
	case Notification:
		return "notification"
	case OutputFlood:
		return "output.flood"
	case RecordingLimitReached:
		return "recording.limit_reached"
	case ShellReady:
		return "shell.ready"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes the event type as its dot-notation string.
func (t Type) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// UnmarshalJSON decodes a dot-notation string back into a Type.
func (t *Type) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("event: type must be a JSON string")
	}
	s := string(data[1 : len(data)-1])
	parsed, ok := typeFromString(s)
	if !ok {
		return fmt.Errorf("event: unknown type %q", s)
	}
	*t = parsed
	return nil
}

// typeFromString maps a dot-notation string to its Type constant.
func typeFromString(s string) (Type, bool) {
	switch s {
	case "session.created":
		return SessionCreated, true
	case "session.attached":
		return SessionAttached, true
	case "session.detached":
		return SessionDetached, true
	case "session.exited":
		return SessionExited, true
	case "session.idle":
		return SessionIdle, true
	case "session.killed":
		return SessionKilled, true
	case "resize":
		return Resize, true
	case "cwd.changed":
		return CwdChanged, true
	case "notification":
		return Notification, true
	case "output.flood":
		return OutputFlood, true
	case "recording.limit_reached":
		return RecordingLimitReached, true
	case "shell.ready":
		return ShellReady, true
	default:
		return 0, false
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
