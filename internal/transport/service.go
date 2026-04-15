package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// randReader is the source of randomness used to generate client IDs.
// It is a package-level variable so tests can inject a failing reader.
var randReader io.Reader = rand.Reader

// Repository defines persistent storage operations for Client records.
type Repository interface {
	// Get retrieves a client by ID.
	Get(id string) (*Client, error)
	// List returns all stored clients.
	List() ([]*Client, error)
	// Save stores or updates a client.
	Save(c *Client) error
	// Delete removes a client by ID.
	Delete(id string) error
}

const (
	// clientIDSize is the number of random bytes used to build a client ID.
	// Hex-encoded, the ID will be 32 characters long.
	clientIDSize = 16

	// handshakeTimeout is the maximum time allowed for the auth handshake.
	handshakeTimeout = 5 * time.Second

	// minAuthPayloadSize is the minimum valid auth payload:
	// 1 byte channel type + auth.TokenSize bytes token.
	minAuthPayloadSize = 1 + auth.TokenSize
)

// Server accepts client connections, performs the auth handshake, and
// maintains the registry of active clients.
type Server struct {
	mu           sync.RWMutex
	listener     *ipc.Listener
	token        []byte
	clients      map[string]*Client
	mode         AutomationMode
	closed       bool
	onClientFunc func(*Client)
}

// NewServer creates a Server that accepts on listener and authenticates
// connections with token. Additional behaviour can be configured with opts.
func NewServer(listener *ipc.Listener, token []byte, opts ...Option) *Server {
	s := &Server{
		mu:           sync.RWMutex{},
		listener:     listener,
		token:        token,
		clients:      make(map[string]*Client),
		mode:         ModeOpen,
		closed:       false,
		onClientFunc: nil,
	}

	for _, o := range opts {
		o(s)
	}

	return s
}

// OnClient registers a callback that fires when a new client completes
// control channel authentication.
func (s *Server) OnClient(fn func(*Client)) {
	s.mu.Lock()
	s.onClientFunc = fn
	s.mu.Unlock()
}

// Serve runs the accept loop until ctx is cancelled or the listener is closed.
// It returns ErrServerClosed when the server shuts down cleanly.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		_ = s.listener.Close()
	}()

	for {
		raw, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()

			if closed || errors.Is(err, net.ErrClosed) {
				return ErrServerClosed
			}

			return fmt.Errorf("transport: accept: %w", err)
		}

		go s.handleConn(raw)
	}
}

// handleConn performs the auth handshake for a single incoming connection.
func (s *Server) handleConn(raw net.Conn) {
	// Enforce handshake deadline to prevent connections that never send auth.
	if err := raw.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		_ = raw.Close()
		return
	}

	creds, err := ipc.ExtractPeerCredentials(raw)
	if err != nil {
		conn := protocol.NewConn(raw)
		sendError(conn, ErrAuthFailed.Error())
		_ = raw.Close()
		return
	}

	conn := protocol.NewConn(raw)

	frame, err := conn.ReadFrame()
	if err != nil {
		sendError(conn, ErrAuthFailed.Error())
		_ = raw.Close()
		return
	}

	if frame.Type != protocol.MsgAuth {
		sendError(conn, ErrAuthFailed.Error())
		_ = raw.Close()
		return
	}

	if len(frame.Payload) < minAuthPayloadSize {
		sendError(conn, ErrAuthFailed.Error())
		_ = raw.Close()
		return
	}

	channelByte := frame.Payload[0]
	tokenBytes := frame.Payload[1 : 1+auth.TokenSize]

	if !auth.Verify(s.token, tokenBytes) {
		sendError(conn, ErrAuthFailed.Error())
		_ = raw.Close()
		return
	}

	if err = s.checkAutomationMode(raw, creds); err != nil {
		sendError(conn, ErrAuthFailed.Error())
		_ = raw.Close()
		return
	}

	// Clear the deadline after a successful handshake.
	if err = raw.SetDeadline(time.Time{}); err != nil {
		_ = raw.Close()
		return
	}

	switch ChannelType(channelByte) {
	case ChannelControl:
		s.registerControl(conn, creds)
	case ChannelStream:
		s.registerStream(conn, frame.Payload, creds)
	default:
		sendError(conn, ErrInvalidChannel.Error())
		_ = raw.Close()
	}
}

// registerControl creates a new Client entry for a control-channel connection
// and sends back a MsgOK frame carrying the assigned client ID.
func (s *Server) registerControl(conn *protocol.Conn, creds ipc.PeerCredentials) {
	id, err := generateClientID()
	if err != nil {
		sendError(conn, "internal error generating client ID")
		_ = conn.Close()
		return
	}

	c := &Client{
		ID:               id,
		Control:          conn,
		Stream:           nil,
		Creds:            creds,
		ConnectedAt:      time.Now(),
		LastHeartbeatAck: time.Time{},
		MissedHeartbeats: 0,
	}

	s.mu.Lock()
	s.clients[id] = c
	s.mu.Unlock()

	if err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgOK,
		Payload: []byte(id),
	}); err != nil {
		s.mu.Lock()
		delete(s.clients, id)
		s.mu.Unlock()
		_ = conn.Close()

		return
	}

	s.mu.RLock()
	cb := s.onClientFunc
	s.mu.RUnlock()

	if cb != nil {
		cb(c)
	}
}

// registerStream associates a stream-channel connection with an existing Client.
// The payload layout after [channel_type:1][token:32] is [client_id_len:1][client_id:N].
func (s *Server) registerStream(conn *protocol.Conn, payload []byte, _ ipc.PeerCredentials) {
	const streamHeaderSize = 1 + auth.TokenSize // channel byte + token

	if len(payload) < streamHeaderSize+2 { // need at least 1-byte len + 1 char ID
		sendError(conn, ErrAuthFailed.Error())
		_ = conn.Close()
		return
	}

	idLen := int(payload[streamHeaderSize])
	if streamHeaderSize+1+idLen > len(payload) {
		sendError(conn, ErrAuthFailed.Error())
		_ = conn.Close()
		return
	}

	clientID := string(payload[streamHeaderSize+1 : streamHeaderSize+1+idLen])

	s.mu.Lock()
	c, ok := s.clients[clientID]
	if !ok {
		s.mu.Unlock()
		sendError(conn, ErrClientNotFound.Error())
		_ = conn.Close()
		return
	}

	if c.Stream != nil {
		s.mu.Unlock()
		sendError(conn, ErrDuplicateChannel.Error())
		_ = conn.Close()
		return
	}

	c.Stream = conn
	s.mu.Unlock()

	if err := conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgOK,
		Payload: nil,
	}); err != nil {
		s.mu.Lock()
		if s.clients[clientID] != nil {
			s.clients[clientID].Stream = nil
		}
		s.mu.Unlock()
		_ = conn.Close()
	}
}

// checkAutomationMode verifies whether the connecting process is permitted
// based on the configured AutomationMode.
func (s *Server) checkAutomationMode(raw net.Conn, creds ipc.PeerCredentials) error {
	s.mu.RLock()
	mode := s.mode
	s.mu.RUnlock()

	switch mode {
	case ModeOpen:
		return nil
	case ModeSameUser:
		if creds.UID != uint32(os.Getuid()) { //nolint:gosec // safe narrowing
			return fmt.Errorf("%w: UID mismatch", ErrAuthFailed)
		}

		return nil
	case ModeChildren:
		selfPID := int32(os.Getpid()) //nolint:gosec // safe narrowing
		if !isDescendant(creds.PID, selfPID) {
			return fmt.Errorf("%w: not a descendant process", ErrAuthFailed)
		}

		return nil
	default:
		_ = raw // suppress unused parameter warning
		return fmt.Errorf("%w: unknown automation mode", ErrAuthFailed)
	}
}

// Clients returns a snapshot of all currently registered clients.
func (s *Server) Clients() []*Client {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Client, 0, len(s.clients))
	for _, c := range s.clients {
		out = append(out, c)
	}

	return out
}

// Client looks up a connected client by ID.
func (s *Server) Client(id string) (*Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.clients[id]
	if !ok {
		return nil, ErrClientNotFound
	}

	return c, nil
}

// Kick closes and removes the client identified by id.
func (s *Server) Kick(id string) error {
	s.mu.Lock()
	c, ok := s.clients[id]
	if !ok {
		s.mu.Unlock()
		return ErrClientNotFound
	}

	delete(s.clients, id)
	s.mu.Unlock()

	if c.Control != nil {
		_ = c.Control.Close()
	}

	if c.Stream != nil {
		_ = c.Stream.Close()
	}

	return nil
}

// Broadcast sends frame to every client that has an active stream channel.
func (s *Server) Broadcast(f protocol.Frame) error {
	s.mu.RLock()
	streams := make([]*protocol.Conn, 0, len(s.clients))
	for _, c := range s.clients {
		if c.Stream != nil {
			streams = append(streams, c.Stream)
		}
	}
	s.mu.RUnlock()

	var first error

	for _, stream := range streams {
		if err := stream.WriteFrame(f); err != nil && first == nil {
			first = fmt.Errorf("transport: broadcast write: %w", err)
		}
	}

	return first
}

// BroadcastTo sends frame to the stream channel of the client identified by id.
func (s *Server) BroadcastTo(clientID string, f protocol.Frame) error {
	s.mu.RLock()
	c, ok := s.clients[clientID]
	s.mu.RUnlock()

	if !ok {
		return ErrClientNotFound
	}

	if c.Stream == nil {
		return nil
	}

	if err := c.Stream.WriteFrame(f); err != nil {
		return fmt.Errorf("transport: broadcast-to write: %w", err)
	}

	return nil
}

// RemoveClient removes the client from the registry without closing connections.
func (s *Server) RemoveClient(id string) {
	s.mu.Lock()
	delete(s.clients, id)
	s.mu.Unlock()
}

// Close shuts down the server and closes all client connections.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	clients := make([]*Client, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, c)
	}
	s.clients = make(map[string]*Client)
	s.mu.Unlock()

	for _, c := range clients {
		if c.Control != nil {
			_ = c.Control.Close()
		}

		if c.Stream != nil {
			_ = c.Stream.Close()
		}
	}

	if err := s.listener.Close(); err != nil {
		return fmt.Errorf("transport: close listener: %w", err)
	}

	return nil
}

// generateClientID creates a random hex-encoded client ID (32 characters).
func generateClientID() (string, error) {
	buf := make([]byte, clientIDSize)
	if _, err := io.ReadFull(randReader, buf); err != nil {
		return "", fmt.Errorf("transport: generate client ID: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

// sendError writes a MsgError frame with msg as payload to conn.
// Errors from WriteFrame are silently ignored because the connection
// is about to be closed.
func sendError(conn *protocol.Conn, msg string) {
	_ = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgError,
		Payload: []byte(msg),
	})
}

// isDescendant reports whether childPID is a descendant of ancestorPID by
// walking the process tree upward via parentPID.
func isDescendant(childPID, ancestorPID int32) bool {
	const maxDepth = 64 // guard against cycles / very deep trees

	current := childPID

	for range maxDepth {
		ppid, err := parentPID(current)
		if err != nil || ppid <= 0 {
			return false
		}

		if ppid == ancestorPID {
			return true
		}

		current = ppid
	}

	return false
}

// parentPID returns the parent process ID of pid by delegating to the
// platform-specific implementation.
func parentPID(pid int32) (int32, error) {
	return platformParentPID(pid)
}
