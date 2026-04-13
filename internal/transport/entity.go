// Package transport manages authenticated client connections to the wmux daemon.
package transport

import (
	"errors"
	"time"

	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// ChannelType identifies whether a connection is a control or stream channel.
type ChannelType byte

const (
	// ChannelControl is the request/response RPC channel.
	ChannelControl ChannelType = 0x01
	// ChannelStream is the push-based broadcast channel.
	ChannelStream ChannelType = 0x02
)

// String returns a human-readable name for the channel type.
func (c ChannelType) String() string {
	switch c {
	case ChannelControl:
		return "control"
	case ChannelStream:
		return "stream"
	default:
		return "unknown"
	}
}

// AutomationMode controls which processes may connect to the daemon.
type AutomationMode int

const (
	// ModeOpen allows any process with a valid token to connect.
	ModeOpen AutomationMode = iota + 1
	// ModeSameUser allows only processes running as the same Unix user.
	ModeSameUser
	// ModeChildren allows only processes spawned by the daemon or its sessions.
	ModeChildren
)

// String returns a human-readable name for the automation mode.
func (m AutomationMode) String() string {
	switch m {
	case ModeOpen:
		return "open"
	case ModeSameUser:
		return "same-user"
	case ModeChildren:
		return "children"
	default:
		return "unknown"
	}
}

// Client represents an authenticated client with its control and stream connections.
type Client struct {
	// ID is the server-assigned unique identifier for this client.
	ID string
	// Control is the request/response connection. May be nil if not yet established.
	Control *protocol.Conn
	// Stream is the push broadcast connection. May be nil if not yet established.
	Stream *protocol.Conn
	// Creds holds the peer credentials extracted from the connection.
	Creds ipc.PeerCredentials
	// ConnectedAt is when the client first authenticated.
	ConnectedAt time.Time
	// LastHeartbeatAck is when the client last acknowledged a heartbeat.
	LastHeartbeatAck time.Time
	// MissedHeartbeats is the number of consecutive missed heartbeat acks.
	MissedHeartbeats int
}

// Sentinel errors for transport operations.
var (
	// ErrAuthFailed is returned when authentication fails (bad token or mode violation).
	ErrAuthFailed = errors.New("transport: authentication failed")
	// ErrClientNotFound is returned when a client ID does not match any connected client.
	ErrClientNotFound = errors.New("transport: client not found")
	// ErrInvalidChannel is returned when a handshake specifies an unknown channel type.
	ErrInvalidChannel = errors.New("transport: invalid channel type")
	// ErrDuplicateChannel is returned when a client tries to register the same channel twice.
	ErrDuplicateChannel = errors.New("transport: duplicate channel")
	// ErrServerClosed is returned when the server has been shut down.
	ErrServerClosed = errors.New("transport: server closed")
)
