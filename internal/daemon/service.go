package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/protocol"
)

const (
	// broadcastInterval is how often the output broadcaster polls sessions.
	broadcastInterval = 16 * time.Millisecond

	// shutdownDelay is how long to wait after sending a shutdown response
	// before actually stopping the server, so the frame can be flushed.
	shutdownDelay = 50 * time.Millisecond
)

// Repository is unused here — kept for goframe compliance. Daemon has no
// persistent repository; it delegates to the SessionManager and Server.
type Repository interface{}

// EventBus abstracts event bus operations for the daemon.
type EventBus interface {
	// Publish sends an event to all subscribers.
	Publish(e event.Event)
	// Subscribe returns a subscription that receives all events.
	Subscribe() *event.Subscription
}

// SessionCreateOptions holds the parameters forwarded to the session backend.
type SessionCreateOptions struct {
	// Shell is the path to the shell binary.
	Shell string
	// Args contains additional arguments for the shell.
	Args []string
	// Cols is the initial terminal width in columns.
	Cols int
	// Rows is the initial terminal height in rows.
	Rows int
	// Cwd is the initial working directory.
	Cwd string
	// Env is the environment variable list.
	Env []string
}

// SessionInfo holds metadata about a managed terminal session.
type SessionInfo struct {
	// ID is the session identifier.
	ID string
	// State is the human-readable lifecycle state.
	State string
	// Pid is the process ID.
	Pid int
	// Cols is the terminal width.
	Cols int
	// Rows is the terminal height.
	Rows int
	// Shell is the shell binary path.
	Shell string
}

// SessionManager abstracts session lifecycle operations.
type SessionManager interface {
	// Create starts a new session with the given id and options.
	Create(id string, opts SessionCreateOptions) (SessionInfo, error)
	// Get returns the SessionInfo for the given id.
	Get(id string) (SessionInfo, error)
	// List returns all current sessions.
	List() []SessionInfo
	// Kill stops the session with the given id.
	Kill(id string) error
	// Resize updates the terminal dimensions of the session.
	Resize(id string, cols, rows int) error
	// WriteInput sends data to the session's PTY.
	WriteInput(id string, data []byte) error
	// ReadOutput drains buffered output from the session.
	ReadOutput(id string) ([]byte, error)
	// Attach transitions the session to attached state.
	Attach(id string) error
	// Detach transitions the session to detached state.
	Detach(id string) error
	// LastActivity returns the time of last PTY output.
	LastActivity(id string) (time.Time, error)
	// Snapshot returns the current terminal screen state for the session.
	Snapshot(id string) (SnapshotData, error)
	// OnExit registers a callback invoked when a session exits.
	OnExit(fn func(id string, exitCode int))
}

// ControlConn abstracts a control-channel connection.
type ControlConn interface {
	// ReadFrame reads one frame from the connection.
	ReadFrame() (protocol.Frame, error)
	// WriteFrame writes one frame to the connection.
	WriteFrame(f protocol.Frame) error
}

// ConnectedClient represents an authenticated client connection.
type ConnectedClient interface {
	// ClientID returns the server-assigned unique identifier.
	ClientID() string
	// Control returns the control-channel connection.
	Control() ControlConn
}

// TransportServer abstracts the transport server.
type TransportServer interface {
	// OnClient registers a callback invoked when a client connects.
	OnClient(fn func(ConnectedClient))
	// Serve runs the accept loop until ctx is cancelled.
	Serve(ctx context.Context) error
	// BroadcastTo sends a frame to the stream channel of the given client.
	BroadcastTo(clientID string, f protocol.Frame) error
}

// Daemon wires a TransportServer to a SessionManager, routes control
// messages, and broadcasts session output to attached clients.
type Daemon struct {
	mu            sync.RWMutex
	server        TransportServer
	sessionSvc    SessionManager
	version       string
	pidFilePath   string
	dataDir       string
	cancelFunc    context.CancelFunc
	attachments   map[string]map[string]struct{} // session_id -> set of client_ids
	clientSession map[string]string              // client_id -> session_id
	eventBus      EventBus                       // may be nil
	startedAt     time.Time
}

// NewDaemon creates a Daemon that uses server for transport and sessionSvc
// for session management. Additional options can be supplied to configure
// the PID file path, version, and data directory.
func NewDaemon(server TransportServer, sessionSvc SessionManager, opts ...Option) *Daemon {
	d := &Daemon{
		mu:            sync.RWMutex{},
		server:        server,
		sessionSvc:    sessionSvc,
		version:       "",
		pidFilePath:   "",
		dataDir:       "",
		cancelFunc:    nil,
		attachments:   make(map[string]map[string]struct{}),
		clientSession: make(map[string]string),
		eventBus:      nil,
		startedAt:     time.Time{},
	}

	for _, o := range opts {
		o(d)
	}

	return d
}

// Start runs the daemon until ctx is cancelled or a MsgShutdown is received.
// It writes the PID file (if configured), registers the OnClient callback,
// starts the output broadcaster, and then blocks on server.Serve.
func (d *Daemon) Start(ctx context.Context) error {
	d.mu.Lock()
	d.startedAt = time.Now()
	d.mu.Unlock()

	if d.pidFilePath != "" {
		info := Info{
			PID:       os.Getpid(),
			Version:   d.version,
			StartedAt: time.Now(),
		}

		if err := WritePIDFile(d.pidFilePath, info); err != nil {
			return fmt.Errorf("daemon: write pid file: %w", err)
		}
	}

	if d.dataDir != "" {
		_, _ = ReconcileOrphans(d.dataDir)
	}

	childCtx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancelFunc = cancel
	d.mu.Unlock()

	d.sessionSvc.OnExit(func(id string, exitCode int) {
		d.publishEvent(event.Event{
			Type:      event.SessionExited,
			SessionID: id,
			Payload:   map[string]any{"exit_code": exitCode},
		})
	})

	d.server.OnClient(func(c ConnectedClient) {
		go d.readControl(childCtx, c)
	})

	go d.broadcastOutput(childCtx)

	err := d.server.Serve(childCtx)

	cancel()

	if d.pidFilePath != "" {
		_ = RemovePIDFile(d.pidFilePath)
	}

	if err != nil {
		return fmt.Errorf("daemon: serve: %w", err)
	}

	return nil
}

// Stop cancels the daemon context, which triggers the server to stop accepting
// new connections and causes Start to return.
func (d *Daemon) Stop() {
	d.mu.RLock()
	fn := d.cancelFunc
	d.mu.RUnlock()

	if fn != nil {
		fn()
	}
}

// publishEvent emits an event if the event bus is configured.
func (d *Daemon) publishEvent(e event.Event) {
	if d.eventBus != nil {
		d.eventBus.Publish(e)
	}
}

// readControl reads control frames from a client connection and dispatches
// them to the appropriate handler until the connection is closed or ctx ends.
func (d *Daemon) readControl(ctx context.Context, c ConnectedClient) {
	for {
		frame, err := c.Control().ReadFrame()
		if err != nil {
			d.detachClient(c.ClientID())
			return
		}

		select {
		case <-ctx.Done():
			d.detachClient(c.ClientID())
			return
		default:
		}

		d.dispatch(c, frame)
	}
}

// dispatch routes a single control frame to the appropriate handler.
func (d *Daemon) dispatch(c ConnectedClient, frame protocol.Frame) {
	switch frame.Type {
	case protocol.MsgCreate:
		d.handleCreate(c, frame)
	case protocol.MsgList:
		d.handleList(c)
	case protocol.MsgInfo:
		d.handleInfo(c, frame)
	case protocol.MsgKill:
		d.handleKill(c, frame)
	case protocol.MsgResize:
		d.handleResize(c, frame)
	case protocol.MsgInput:
		d.handleInput(c, frame)
	case protocol.MsgAttach:
		d.handleAttach(c, frame)
	case protocol.MsgDetach:
		d.handleDetach(c, frame)
	case protocol.MsgShutdown:
		d.handleShutdown(c)
	case protocol.MsgEvent:
		d.handleSubscribe(c, frame)
	case protocol.MsgStatus:
		d.handleStatus(c)
	default:
		_ = c.Control().WriteFrame(errorFrame("unknown message type"))
	}
}

// handleCreate processes a MsgCreate frame.
func (d *Daemon) handleCreate(c ConnectedClient, frame protocol.Frame) {
	var req CreateRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid create request"))
		return
	}

	info, err := d.sessionSvc.Create(req.ID, SessionCreateOptions{
		Shell: req.Shell,
		Args:  req.Args,
		Cols:  req.Cols,
		Rows:  req.Rows,
		Cwd:   req.Cwd,
		Env:   req.Env,
	})
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	_ = c.Control().WriteFrame(okFrame(sessionInfoToResponse(info)))

	d.publishEvent(event.Event{
		Type:      event.SessionCreated,
		SessionID: req.ID,
		Payload:   map[string]any{"shell": req.Shell, "pid": info.Pid},
	})
}

// handleList processes a MsgList frame.
func (d *Daemon) handleList(c ConnectedClient) {
	sessions := d.sessionSvc.List()
	resps := make([]SessionResponse, 0, len(sessions))

	for _, info := range sessions {
		resps = append(resps, sessionInfoToResponse(info))
	}

	_ = c.Control().WriteFrame(okFrame(resps))
}

// handleInfo processes a MsgInfo frame.
func (d *Daemon) handleInfo(c ConnectedClient, frame protocol.Frame) {
	var req SessionIDRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid info request"))
		return
	}

	info, err := d.sessionSvc.Get(req.SessionID)
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	_ = c.Control().WriteFrame(okFrame(sessionInfoToResponse(info)))
}

// handleKill processes a MsgKill frame.
func (d *Daemon) handleKill(c ConnectedClient, frame protocol.Frame) {
	var req SessionIDRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid kill request"))
		return
	}

	if err := d.sessionSvc.Kill(req.SessionID); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleResize processes a MsgResize frame.
func (d *Daemon) handleResize(c ConnectedClient, frame protocol.Frame) {
	var req ResizeRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid resize request"))
		return
	}

	if err := d.sessionSvc.Resize(req.SessionID, req.Cols, req.Rows); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleInput processes a MsgInput frame using binary encoding.
func (d *Daemon) handleInput(c ConnectedClient, frame protocol.Frame) {
	sessionID, data, err := DecodeInputPayload(frame.Payload)
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid input payload"))
		return
	}

	if err = d.sessionSvc.WriteInput(sessionID, data); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleAttach records the attachment of a client to a session and transitions
// the session to attached state.
func (d *Daemon) handleAttach(c ConnectedClient, frame protocol.Frame) {
	var req SessionIDRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid attach request"))
		return
	}

	info, err := d.sessionSvc.Get(req.SessionID)
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	if err := d.sessionSvc.Attach(req.SessionID); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	d.mu.Lock()
	if _, ok := d.attachments[req.SessionID]; !ok {
		d.attachments[req.SessionID] = make(map[string]struct{})
	}
	d.attachments[req.SessionID][c.ClientID()] = struct{}{}
	d.clientSession[c.ClientID()] = req.SessionID
	d.mu.Unlock()

	resp := AttachResponse{
		ID:    info.ID,
		State: info.State,
		Pid:   info.Pid,
		Cols:  info.Cols,
		Rows:  info.Rows,
		Shell: info.Shell,
	}

	snap, snapErr := d.sessionSvc.Snapshot(req.SessionID)
	if snapErr == nil && (len(snap.Scrollback) > 0 || len(snap.Viewport) > 0) {
		resp.Snapshot = &SnapshotResponse{
			Scrollback: snap.Scrollback,
			Viewport:   snap.Viewport,
		}
	}

	_ = c.Control().WriteFrame(okFrame(resp))

	d.publishEvent(event.Event{
		Type:      event.SessionAttached,
		SessionID: req.SessionID,
		Payload:   map[string]any{"client_id": c.ClientID()},
	})
}

// handleDetach removes the attachment of a client from a session and transitions
// the session to detached state when the last client leaves.
func (d *Daemon) handleDetach(c ConnectedClient, frame protocol.Frame) {
	var req SessionIDRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid detach request"))
		return
	}

	d.mu.Lock()
	shouldDetach := false
	if clients, ok := d.attachments[req.SessionID]; ok {
		delete(clients, c.ClientID())
		if len(clients) == 0 {
			delete(d.attachments, req.SessionID)
			shouldDetach = true
		}
	}
	delete(d.clientSession, c.ClientID())
	d.mu.Unlock()

	if shouldDetach {
		_ = d.sessionSvc.Detach(req.SessionID)
		d.publishEvent(event.Event{
			Type:      event.SessionDetached,
			SessionID: req.SessionID,
			Payload:   nil,
		})
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleShutdown responds OK and then stops the daemon after a short delay.
func (d *Daemon) handleShutdown(c ConnectedClient) {
	_ = c.Control().WriteFrame(okFrame(nil))

	go func() {
		time.Sleep(shutdownDelay)
		d.Stop()
	}()
}

// detachClient removes all attachment state for the disconnected client and
// transitions the session to detached state when this was the last client.
func (d *Daemon) detachClient(clientID string) {
	d.mu.Lock()

	sessID, ok := d.clientSession[clientID]
	if !ok {
		d.mu.Unlock()
		return
	}

	delete(d.clientSession, clientID)

	shouldDetach := false
	if clients, ok := d.attachments[sessID]; ok {
		delete(clients, clientID)
		if len(clients) == 0 {
			delete(d.attachments, sessID)
			shouldDetach = true
		}
	}

	d.mu.Unlock()

	if shouldDetach {
		_ = d.sessionSvc.Detach(sessID)
		d.publishEvent(event.Event{
			Type:      event.SessionDetached,
			SessionID: sessID,
			Payload:   nil,
		})
	}
}

// broadcastOutput polls every session for new output and sends MsgData
// frames to all attached clients at broadcastInterval.
func (d *Daemon) broadcastOutput(ctx context.Context) {
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.flushOutput()
		}
	}
}

// flushOutput reads pending output from every session and broadcasts it to
// any clients that are currently attached to that session.
func (d *Daemon) flushOutput() {
	d.mu.RLock()
	sessions := make(map[string]map[string]struct{}, len(d.attachments))

	for sessID, clients := range d.attachments {
		snapshot := make(map[string]struct{}, len(clients))
		for id := range clients {
			snapshot[id] = struct{}{}
		}

		sessions[sessID] = snapshot
	}
	d.mu.RUnlock()

	for sessID, clients := range sessions {
		data, err := d.sessionSvc.ReadOutput(sessID)
		if err != nil || len(data) == 0 {
			continue
		}

		payload := EncodeDataPayload(sessID, data)
		frame := protocol.Frame{
			Version: protocol.ProtocolVersion,
			Type:    protocol.MsgData,
			Payload: payload,
		}

		for clientID := range clients {
			_ = d.server.BroadcastTo(clientID, frame)
		}
	}
}

// handleStatus processes a MsgStatus frame and returns daemon health info.
func (d *Daemon) handleStatus(c ConnectedClient) {
	d.mu.RLock()
	startedAt := d.startedAt
	clientCount := len(d.clientSession)
	d.mu.RUnlock()

	sessions := d.sessionSvc.List()

	resp := StatusResponse{
		Version:      d.version,
		Uptime:       time.Since(startedAt).Truncate(time.Second).String(),
		SessionCount: len(sessions),
		ClientCount:  clientCount,
	}

	_ = c.Control().WriteFrame(okFrame(resp))
}

// handleSubscribe processes a MsgEvent frame to subscribe clients to events.
func (d *Daemon) handleSubscribe(c ConnectedClient, frame protocol.Frame) {
	if d.eventBus == nil {
		_ = c.Control().WriteFrame(errorFrame("events not enabled"))
		return
	}

	var req EventSubscribeRequest
	if len(frame.Payload) > 0 {
		if err := json.Unmarshal(frame.Payload, &req); err != nil {
			_ = c.Control().WriteFrame(errorFrame("invalid event subscribe request"))
			return
		}
	}

	sub := d.eventBus.Subscribe()

	_ = c.Control().WriteFrame(okFrame(nil))

	// Forward events to client's control channel in a goroutine.
	go func() {
		defer sub.Unsubscribe()
		for evt := range sub.Events() {
			if req.SessionID != "" && evt.SessionID != req.SessionID {
				continue
			}

			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}

			err = c.Control().WriteFrame(protocol.Frame{
				Version: protocol.ProtocolVersion,
				Type:    protocol.MsgEvent,
				Payload: data,
			})
			if err != nil {
				return // client disconnected
			}
		}
	}()
}

// okFrame builds a MsgOK frame with an optional JSON payload.
// If payload is nil, the frame carries no data.
func okFrame(payload any) protocol.Frame {
	if payload == nil {
		return protocol.Frame{
			Version: protocol.ProtocolVersion,
			Type:    protocol.MsgOK,
			Payload: nil,
		}
	}

	data, _ := json.Marshal(payload) //nolint:errcheck

	return protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgOK,
		Payload: data,
	}
}

// errorFrame builds a MsgError frame carrying an ErrorResponse JSON payload.
func errorFrame(msg string) protocol.Frame {
	data, _ := json.Marshal(ErrorResponse{Error: msg}) //nolint:errcheck

	return protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgError,
		Payload: data,
	}
}

// sessionInfoToResponse converts a SessionInfo to a SessionResponse.
func sessionInfoToResponse(info SessionInfo) SessionResponse {
	return SessionResponse(info)
}
