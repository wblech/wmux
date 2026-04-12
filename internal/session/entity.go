// Package session defines the core domain types for terminal session management.
package session

import (
	"errors"
	"time"
)

// State represents the lifecycle state of a session.
type State int

const (
	_ State = iota
	// StateAlive indicates the session is running and attached.
	StateAlive
	// StateDetached indicates the session is running but not attached.
	StateDetached
	// StateExited indicates the session process has exited.
	StateExited
	// StateRemoved indicates the session has been fully removed.
	StateRemoved
)

// String returns a human-readable representation of the state.
func (s State) String() string {
	switch s {
	case StateAlive:
		return "alive"
	case StateDetached:
		return "detached"
	case StateExited:
		return "exited"
	case StateRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// IsTerminal reports whether the state is a terminal state,
// meaning the session will not transition to any other state.
func (s State) IsTerminal() bool {
	return s == StateExited || s == StateRemoved
}

// Snapshot holds a point-in-time capture of a terminal screen.
type Snapshot struct {
	// Scrollback contains the lines that have scrolled off the visible viewport.
	Scrollback []byte
	// Viewport contains the currently visible terminal content.
	Viewport []byte
}

// ScreenEmulator processes terminal output and provides screen snapshots.
type ScreenEmulator interface {
	// Process handles incoming terminal data bytes.
	Process(data []byte)
	// Snapshot returns the current terminal screen state.
	Snapshot() Snapshot
	// Resize updates the terminal dimensions.
	Resize(cols, rows int)
}

// Session holds the state and metadata for a managed terminal session.
type Session struct {
	// ID is the unique identifier for the session.
	ID string
	// State is the current lifecycle state of the session.
	State State
	// Pid is the process ID of the shell running inside the session.
	Pid int
	// Cols is the terminal width in columns.
	Cols int
	// Rows is the terminal height in rows.
	Rows int
	// Shell is the path to the shell binary.
	Shell string
	// Cwd is the current working directory of the session.
	Cwd string
	// ExitCode is the exit code of the shell process after it exits.
	ExitCode int
	// StartedAt is the time when the session was created.
	StartedAt time.Time
	// EndedAt is the time when the session ended (zero if still running).
	EndedAt time.Time
}

// Sentinel errors for session operations.
var (
	// ErrSessionNotFound is returned when the requested session does not exist.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExists is returned when creating a session with a duplicate ID.
	ErrSessionExists = errors.New("session already exists")
	// ErrInvalidState is returned when a state transition is not allowed.
	ErrInvalidState = errors.New("invalid session state")
	// ErrInvalidSessionID is returned when a session ID fails validation.
	ErrInvalidSessionID = errors.New("invalid session ID")
	// ErrMaxSessions is returned when the session limit has been reached.
	ErrMaxSessions = errors.New("maximum number of sessions reached")
	// ErrBufferFull is returned when the session output buffer is full.
	ErrBufferFull = errors.New("session buffer full")
)
