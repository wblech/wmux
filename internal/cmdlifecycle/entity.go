// Package cmdlifecycle tracks the per-session command lifecycle driven by
// OSC 133 shell-integration markers. Pure-logic domain package with no I/O;
// persistence is delegated to a Repository implementation injected by the
// daemon.
package cmdlifecycle

import (
	"errors"
	"time"
)

// State represents the per-session command lifecycle phase.
type State int

const (
	// StateIdle indicates no in-flight command and no prompt yet.
	StateIdle State = iota
	// StatePromptShown indicates the shell drew a prompt (OSC 133;A).
	StatePromptShown
	// StateUserTyping indicates the user has begun typing (OSC 133;B).
	StateUserTyping
	// StateCommandRunning indicates a command is executing (OSC 133;C).
	StateCommandRunning
)

// String returns a stable lower_snake representation of the state.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePromptShown:
		return "prompt_shown"
	case StateUserTyping:
		return "user_typing"
	case StateCommandRunning:
		return "command_running"
	default:
		return "unknown"
	}
}

// Command captures one completed shell command's lifecycle.
//
// Orphan commands have ExitCode = -1 and a non-empty OrphanReason. They
// are produced when the state machine detects a missing OSC 133;D marker
// or when cold-restore observes an in-flight command after a daemon
// restart.
type Command struct {
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
	ExitCode     int       `json:"exit_code"`
	StartRow     int64     `json:"start_row"`
	EndRow       int64     `json:"end_row"`
	Orphan       bool      `json:"orphan,omitempty"`
	OrphanReason string    `json:"orphan_reason,omitempty"`
}

// SessionState is the snapshot returned by Service.Snapshot and consumed
// by Service.Restore. History is a deep copy of the per-session FIFO ring;
// InFlight is non-nil iff the tracker was in StateCommandRunning at
// snapshot time.
type SessionState struct {
	History  []Command `json:"history"`
	InFlight *Command  `json:"in_flight,omitempty"`
}

// Orphan reason constants identify why a Command was closed as orphan.
const (
	// OrphanReasonMissingD indicates the state machine saw an A/B/C
	// transition while in StateCommandRunning without an intervening D.
	OrphanReasonMissingD = "missing_d_marker"
	// OrphanReasonDaemonRestart indicates an in-flight command was
	// observed at cold-restore — the command was definitionally aborted
	// by daemon shutdown.
	OrphanReasonDaemonRestart = "daemon_restart"
)

// Sentinel errors.
var (
	// ErrSessionNotRegistered is returned by Service methods called for an
	// unknown session ID.
	ErrSessionNotRegistered = errors.New("cmdlifecycle: session not registered")
	// ErrSessionExists is returned by Register or Restore when the session
	// is already known to the Service.
	ErrSessionExists = errors.New("cmdlifecycle: session already registered")
	// ErrInvalidOSCCode is returned by HandleOSC when the code byte is not
	// one of 'A', 'B', 'C', 'D'.
	ErrInvalidOSCCode = errors.New("cmdlifecycle: invalid OSC 133 code")
)
