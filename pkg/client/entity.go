// Package client provides a Go client library for the wmux daemon.
package client

// CreateParams holds parameters for creating a new session.
type CreateParams struct {
	// Shell is the path to the shell binary.
	Shell string
	// Args contains additional shell arguments.
	Args []string
	// Cols is the initial terminal width.
	Cols int
	// Rows is the initial terminal height.
	Rows int
	// Cwd is the initial working directory.
	Cwd string
	// Env is the environment variable list.
	Env []string
}

// SessionInfo holds metadata about a session returned by the daemon.
type SessionInfo struct {
	// ID is the unique session identifier.
	ID string `json:"id"`
	// State is the human-readable lifecycle state.
	State string `json:"state"`
	// Pid is the shell process ID.
	Pid int `json:"pid"`
	// Cols is the terminal width.
	Cols int `json:"cols"`
	// Rows is the terminal height.
	Rows int `json:"rows"`
	// Shell is the shell binary path.
	Shell string `json:"shell"`
}

// Snapshot holds the terminal state captured at attach time.
type Snapshot struct {
	// Scrollback contains lines scrolled off the viewport.
	Scrollback []byte `json:"scrollback"`
	// Viewport contains the visible terminal content.
	Viewport []byte `json:"viewport"`
}

// AttachResult holds the session info and snapshot from an attach operation.
type AttachResult struct {
	// Session is the session metadata.
	Session SessionInfo
	// Snapshot is the terminal state (empty if backend is "none").
	Snapshot Snapshot
}

// SessionHistory holds restored session data from disk (cold restore).
type SessionHistory struct {
	// Scrollback is the raw PTY output from the previous session.
	Scrollback []byte
	// SessionID is the session identifier.
	SessionID string
	// Shell is the shell binary path.
	Shell string
	// Cwd is the working directory at session creation.
	Cwd string
	// Cols is the terminal width at creation.
	Cols int
	// Rows is the terminal height at creation.
	Rows int
}

// Event represents a daemon event delivered to the client.
type Event struct {
	// Type is the event type string (e.g., "session.created").
	Type string `json:"type"`
	// SessionID is the session this event relates to.
	SessionID string `json:"session_id"`
	// Data contains event-specific key-value pairs.
	Data map[string]any `json:"payload,omitempty"`
}
