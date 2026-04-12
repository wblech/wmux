// Package daemon provides the central wmux daemon service that coordinates
// the transport and session layers.
package daemon

import (
	"errors"
	"time"
)

// Info holds metadata about a running daemon instance, persisted to the PID file.
type Info struct {
	PID       int       `json:"pid"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
}

// CreateRequest is the JSON payload for MsgCreate.
type CreateRequest struct {
	ID    string   `json:"id"`
	Shell string   `json:"shell"`
	Args  []string `json:"args,omitempty"`
	Cols  int      `json:"cols"`
	Rows  int      `json:"rows"`
	Cwd   string   `json:"cwd,omitempty"`
	Env   []string `json:"env,omitempty"`
}

// SessionResponse is the JSON payload returned for session operations.
type SessionResponse struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Pid   int    `json:"pid"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	Shell string `json:"shell"`
}

// SessionIDRequest is the JSON payload for commands that target a session by ID.
type SessionIDRequest struct {
	SessionID string `json:"session_id"`
}

// ResizeRequest is the JSON payload for MsgResize.
type ResizeRequest struct {
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// ErrorResponse is the JSON payload for MsgError.
type ErrorResponse struct {
	Error string `json:"error"`
}

// StatusResponse is the JSON payload for MsgStatus responses.
type StatusResponse struct {
	Version      string `json:"version"`
	Uptime       string `json:"uptime"`
	SessionCount int    `json:"session_count"`
	ClientCount  int    `json:"client_count"`
}

// EventSubscribeRequest is the JSON payload for MsgEvent subscription.
type EventSubscribeRequest struct {
	// SessionID filters events to a specific session. Empty means all sessions.
	SessionID string `json:"session_id,omitempty"`
}

// Sentinel errors for daemon operations.
var (
	// ErrDaemonRunning is returned when trying to start a daemon that is already running.
	ErrDaemonRunning = errors.New("daemon: already running")
	// ErrDaemonNotRunning is returned when an operation requires a running daemon but none exists.
	ErrDaemonNotRunning = errors.New("daemon: not running")
	// ErrAlreadyAttached is returned when a client attempts to attach to a session it is already attached to.
	ErrAlreadyAttached = errors.New("daemon: client already attached to session")
	// ErrNotAttached is returned when a client attempts an operation that requires an active attachment.
	ErrNotAttached = errors.New("daemon: client not attached")
	// ErrSessionNotSpecified is returned when a session ID is required but not provided.
	ErrSessionNotSpecified = errors.New("daemon: session ID not specified")
)
