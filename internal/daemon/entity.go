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

// AttachResponse is the JSON payload for MsgAttach responses with snapshot.
type AttachResponse struct {
	ID       string            `json:"id"`
	State    string            `json:"state"`
	Pid      int               `json:"pid"`
	Cols     int               `json:"cols"`
	Rows     int               `json:"rows"`
	Shell    string            `json:"shell"`
	Snapshot *SnapshotResponse `json:"snapshot,omitempty"`
}

// SnapshotResponse carries the two-phase snapshot data.
type SnapshotResponse struct {
	Scrollback []byte `json:"scrollback"`
	Viewport   []byte `json:"viewport"`
}

// SnapshotData holds raw terminal snapshot data returned by SessionManager.
type SnapshotData struct {
	Scrollback []byte
	Viewport   []byte
}

// MetaSetRequest is the JSON payload for MsgMetaSet.
type MetaSetRequest struct {
	SessionID string `json:"session_id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

// MetaGetRequest is the JSON payload for MsgMetaGet.
type MetaGetRequest struct {
	SessionID string `json:"session_id"`
	Key       string `json:"key"` // empty = get all
}

// MetaGetResponse is the JSON payload for MsgMetaGet responses.
type MetaGetResponse struct {
	Value    string            `json:"value,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EnvForwardRequest is the JSON payload for MsgEnvForward.
type EnvForwardRequest struct {
	SessionID string            `json:"session_id"`
	Env       map[string]string `json:"env"`
}

// ExecRequest is the JSON payload for MsgExec.
type ExecRequest struct {
	SessionID string `json:"session_id"`
	Input     string `json:"input"`
	Newline   bool   `json:"newline"`
}

// ExecSyncRequest is the JSON payload for MsgExecSync.
type ExecSyncRequest struct {
	SessionIDs []string `json:"session_ids,omitempty"`
	Prefix     string   `json:"prefix,omitempty"`
	Input      string   `json:"input"`
	Newline    bool     `json:"newline"`
}

// ExecSyncResponse is the JSON payload for MsgExecSync responses.
type ExecSyncResponse struct {
	Results []ExecResult `json:"results"`
}

// ExecResult represents the outcome of an exec operation on a single session.
type ExecResult struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// WaitRequest is the JSON payload for MsgWait.
type WaitRequest struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"`
	Timeout   int64  `json:"timeout"`
	IdleFor   int64  `json:"idle_for,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

// WaitResponse is the JSON payload for MsgWait responses.
type WaitResponse struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"`
	ExitCode  *int   `json:"exit_code"`
	Matched   bool   `json:"matched"`
	TimedOut  bool   `json:"timed_out"`
}

// RecordRequest is the JSON payload for MsgRecord.
type RecordRequest struct {
	SessionID string `json:"session_id"`
	Action    string `json:"action"` // "start" or "stop"
}

// RecordResponse is the JSON payload for MsgRecord responses.
type RecordResponse struct {
	SessionID string `json:"session_id"`
	Recording bool   `json:"recording"`
	Path      string `json:"path,omitempty"`
}

// ListRequest is the optional JSON payload for MsgList.
// When empty or nil payload, all sessions are returned.
type ListRequest struct {
	Prefix string `json:"prefix,omitempty"`
}

// UpdateEmulatorScrollbackRequest is the payload for MsgUpdateEmulatorScrollback.
type UpdateEmulatorScrollbackRequest struct {
	SessionID       string `json:"session_id"`
	ScrollbackLines int    `json:"scrollback_lines"`
}

// KillPrefixRequest is the JSON payload for MsgKillPrefix.
type KillPrefixRequest struct {
	Prefix string `json:"prefix"`
}

// KillPrefixResponse is the JSON payload for MsgKillPrefix responses.
type KillPrefixResponse struct {
	Killed []string          `json:"killed"`
	Errors map[string]string `json:"errors,omitempty"`
}

// HistoryRequest is the JSON payload for MsgHistory.
type HistoryRequest struct {
	SessionID string `json:"session_id"`
	Format    string `json:"format"`          // "ansi", "text", "html"
	Lines     int    `json:"lines,omitempty"` // 0 = all available
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
