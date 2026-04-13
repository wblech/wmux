package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/history"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// logErr logs a non-fatal error to stderr. Intended for best-effort operations
// where the error should be observable but does not affect the response.
func logErr(context string, err error) {
	fmt.Fprintf(os.Stderr, "wmux: %s: %v\n", context, err)
}

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
	// MetaSet sets a metadata key-value pair on a session.
	MetaSet(id, key, value string) error
	// MetaGet returns a metadata value for a session.
	MetaGet(id, key string) (string, error)
	// MetaGetAll returns all metadata for a session.
	MetaGetAll(id string) (map[string]string, error)
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
	mu                sync.RWMutex
	server            TransportServer
	sessionSvc        SessionManager
	version           string
	pidFilePath       string
	dataDir           string
	cancelFunc        context.CancelFunc
	attachments       map[string]map[string]struct{} // session_id -> set of client_ids
	clientSession     map[string]string              // client_id -> session_id
	eventBus          EventBus                       // may be nil
	startedAt         time.Time
	coldRestore       bool
	maxScrollbackSize int64
	scrollbackWriters map[string]*history.Writer // session_id -> writer (cold restore only)
	waiters           map[string][]*waiter       // session_id -> list of active waiters
}

// NewDaemon creates a Daemon that uses server for transport and sessionSvc
// for session management. Additional options can be supplied to configure
// the PID file path, version, and data directory.
func NewDaemon(server TransportServer, sessionSvc SessionManager, opts ...Option) *Daemon {
	d := &Daemon{
		mu:                sync.RWMutex{},
		server:            server,
		sessionSvc:        sessionSvc,
		version:           "",
		pidFilePath:       "",
		dataDir:           "",
		cancelFunc:        nil,
		attachments:       make(map[string]map[string]struct{}),
		clientSession:     make(map[string]string),
		eventBus:          nil,
		startedAt:         time.Time{},
		coldRestore:       false,
		maxScrollbackSize: 0,
		scrollbackWriters: make(map[string]*history.Writer),
		waiters:           make(map[string][]*waiter),
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
		d.notifyExitWaiters(id, exitCode)
		d.notifyExitOnNonExitWaiters(id, exitCode)
		d.persistSessionExit(id, exitCode)
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
	case protocol.MsgMetaSet:
		d.handleMetaSet(c, frame)
	case protocol.MsgMetaGet:
		d.handleMetaGet(c, frame)
	case protocol.MsgEnvForward:
		d.handleEnvForward(c, frame)
	case protocol.MsgExec:
		d.handleExec(c, frame)
	case protocol.MsgExecSync:
		d.handleExecSync(c, frame)
	case protocol.MsgWait:
		go d.handleWait(c, frame)
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

	d.persistSessionCreate(req.ID, req)

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
		ID:       info.ID,
		State:    info.State,
		Pid:      info.Pid,
		Cols:     info.Cols,
		Rows:     info.Rows,
		Shell:    info.Shell,
		Snapshot: nil,
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

		d.scanOSC(sessID, data)
		d.persistOutput(sessID, data)
		d.notifyOutputWaiters(sessID, data)

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

	// Flush output for sessions with active waiters but no attached clients.
	d.mu.RLock()
	waiterSessions := make([]string, 0)
	for sessID, ws := range d.waiters {
		if len(ws) > 0 {
			if _, attached := sessions[sessID]; !attached {
				waiterSessions = append(waiterSessions, sessID)
			}
		}
	}
	d.mu.RUnlock()

	for _, sessID := range waiterSessions {
		data, err := d.sessionSvc.ReadOutput(sessID)
		if err != nil || len(data) == 0 {
			continue
		}

		d.scanOSC(sessID, data)
		d.persistOutput(sessID, data)
		d.notifyOutputWaiters(sessID, data)
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

// scanOSC inspects output data for OSC sequences and emits events / updates metadata.
func (d *Daemon) scanOSC(sessID string, data []byte) {
	results := ParseOSC(data)
	for _, osc := range results {
		switch osc.Type {
		case OSCTypeCwd:
			_ = d.sessionSvc.MetaSet(sessID, "cwd", osc.Value)
			d.publishEvent(event.Event{
				Type:      event.CwdChanged,
				SessionID: sessID,
				Payload:   map[string]any{"cwd": osc.Value},
			})
		case OSCTypeNotification:
			d.publishEvent(event.Event{
				Type:      event.Notification,
				SessionID: sessID,
				Payload:   map[string]any{"body": osc.Value},
			})
		}
	}
}

// handleMetaSet processes a MsgMetaSet frame.
func (d *Daemon) handleMetaSet(c ConnectedClient, frame protocol.Frame) {
	var req MetaSetRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid meta set request"))
		return
	}
	if err := d.sessionSvc.MetaSet(req.SessionID, req.Key, req.Value); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}
	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleMetaGet processes a MsgMetaGet frame.
func (d *Daemon) handleMetaGet(c ConnectedClient, frame protocol.Frame) {
	var req MetaGetRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid meta get request"))
		return
	}
	if req.Key == "" {
		meta, err := d.sessionSvc.MetaGetAll(req.SessionID)
		if err != nil {
			_ = c.Control().WriteFrame(errorFrame(err.Error()))
			return
		}
		_ = c.Control().WriteFrame(okFrame(MetaGetResponse{Value: "", Metadata: meta}))
		return
	}
	val, err := d.sessionSvc.MetaGet(req.SessionID, req.Key)
	if err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}
	_ = c.Control().WriteFrame(okFrame(MetaGetResponse{Value: val, Metadata: nil}))
}

// handleEnvForward processes a MsgEnvForward frame.
func (d *Daemon) handleEnvForward(c ConnectedClient, frame protocol.Frame) {
	var req EnvForwardRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid env forward request"))
		return
	}

	if d.dataDir == "" {
		_ = c.Control().WriteFrame(okFrame(nil))
		return
	}

	sessionDir := filepath.Join(d.dataDir, req.SessionID)
	_ = os.MkdirAll(sessionDir, 0755) //nolint:gosec

	nonPathEnv := make(map[string]string)
	for k, v := range req.Env {
		if _, err := os.Stat(v); err == nil {
			if err := ForwardEnv(sessionDir, k, v); err != nil {
				logErr("env forward symlink", err)
			}
		} else {
			nonPathEnv[k] = v
		}
	}
	if len(nonPathEnv) > 0 {
		if err := WriteEnvFile(sessionDir, nonPathEnv); err != nil {
			logErr("env forward write", err)
		}
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleExec processes a MsgExec frame — sends input to a session without attaching.
func (d *Daemon) handleExec(c ConnectedClient, frame protocol.Frame) {
	var req ExecRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid exec request"))
		return
	}

	input := []byte(req.Input)
	if req.Newline {
		input = append(input, '\n')
	}

	if err := d.sessionSvc.WriteInput(req.SessionID, input); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	_ = c.Control().WriteFrame(okFrame(nil))
}

// handleExecSync processes a MsgExecSync frame — sends input to multiple sessions.
func (d *Daemon) handleExecSync(c ConnectedClient, frame protocol.Frame) {
	var req ExecSyncRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid exec_sync request"))
		return
	}

	if len(req.SessionIDs) > 0 && req.Prefix != "" {
		_ = c.Control().WriteFrame(errorFrame("specify session_ids or prefix, not both"))
		return
	}

	targets := req.SessionIDs
	if req.Prefix != "" {
		targets = d.sessionsByPrefix(req.Prefix)
	}

	if len(targets) == 0 {
		_ = c.Control().WriteFrame(errorFrame("no matching sessions"))
		return
	}

	input := []byte(req.Input)
	if req.Newline {
		input = append(input, '\n')
	}

	results := make([]ExecResult, 0, len(targets))
	for _, sid := range targets {
		if err := d.sessionSvc.WriteInput(sid, input); err != nil {
			results = append(results, ExecResult{SessionID: sid, OK: false, Error: err.Error()})
		} else {
			results = append(results, ExecResult{SessionID: sid, OK: true, Error: ""})
		}
	}

	_ = c.Control().WriteFrame(okFrame(ExecSyncResponse{Results: results}))
}

// sessionsByPrefix returns session IDs matching the given prefix.
func (d *Daemon) sessionsByPrefix(prefix string) []string {
	sessions := d.sessionSvc.List()
	pfx := prefix + "/"
	var matched []string
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, pfx) {
			matched = append(matched, s.ID)
		}
	}
	return matched
}

// waiter represents a single wait condition registered on a session.
type waiter struct {
	mode      string
	pattern   []byte
	idleFor   time.Duration
	idleTimer *time.Timer
	done      chan WaitResponse
	cleanup   func()
}

func (d *Daemon) addWaiter(sessionID string, w *waiter) {
	d.mu.Lock()
	d.waiters[sessionID] = append(d.waiters[sessionID], w)
	d.mu.Unlock()
}

func (d *Daemon) removeWaiter(sessionID string, w *waiter) {
	d.mu.Lock()
	defer d.mu.Unlock()

	waiters := d.waiters[sessionID]
	for i, existing := range waiters {
		if existing == w {
			d.waiters[sessionID] = append(waiters[:i], waiters[i+1:]...)
			return
		}
	}
}

func (d *Daemon) notifyExitWaiters(sessionID string, exitCode int) {
	d.mu.RLock()
	waiters := make([]*waiter, len(d.waiters[sessionID]))
	copy(waiters, d.waiters[sessionID])
	d.mu.RUnlock()

	for _, w := range waiters {
		if w.mode == "exit" {
			ec := exitCode
			select {
			case w.done <- WaitResponse{
				SessionID: sessionID,
				Mode:      "exit",
				ExitCode:  &ec,
				Matched:   false,
				TimedOut:  false,
			}:
			default:
			}
		}
	}
}

func (d *Daemon) notifyOutputWaiters(sessionID string, data []byte) {
	d.mu.RLock()
	waiters := make([]*waiter, len(d.waiters[sessionID]))
	copy(waiters, d.waiters[sessionID])
	d.mu.RUnlock()

	for _, w := range waiters {
		switch w.mode {
		case "match":
			if bytes.Contains(data, w.pattern) {
				select {
				case w.done <- WaitResponse{
					SessionID: sessionID,
					Mode:      "match",
					ExitCode:  nil,
					Matched:   true,
					TimedOut:  false,
				}:
				default:
				}
			}
		case "idle":
			if w.idleTimer != nil {
				w.idleTimer.Reset(w.idleFor)
			}
		}
	}
}

func (d *Daemon) notifyExitOnNonExitWaiters(sessionID string, exitCode int) {
	d.mu.RLock()
	waiters := make([]*waiter, len(d.waiters[sessionID]))
	copy(waiters, d.waiters[sessionID])
	d.mu.RUnlock()

	ec := exitCode
	for _, w := range waiters {
		if w.mode == "idle" || w.mode == "match" {
			select {
			case w.done <- WaitResponse{
				SessionID: sessionID,
				Mode:      w.mode,
				ExitCode:  &ec,
				Matched:   false,
				TimedOut:  false,
			}:
			default:
			}
		}
	}
}

func (d *Daemon) handleWait(c ConnectedClient, frame protocol.Frame) {
	var req WaitRequest
	if err := json.Unmarshal(frame.Payload, &req); err != nil {
		_ = c.Control().WriteFrame(errorFrame("invalid wait request"))
		return
	}

	if _, err := d.sessionSvc.Get(req.SessionID); err != nil {
		_ = c.Control().WriteFrame(errorFrame(err.Error()))
		return
	}

	switch req.Mode {
	case "exit":
		d.waitUntilExit(c, req)
	case "idle":
		d.waitUntilIdle(c, req)
	case "match":
		d.waitUntilMatch(c, req)
	default:
		_ = c.Control().WriteFrame(errorFrame("invalid wait mode: " + req.Mode))
	}
}

func (d *Daemon) waitUntilExit(c ConnectedClient, req WaitRequest) {
	w := &waiter{
		mode:      "exit",
		pattern:   nil,
		idleFor:   0,
		idleTimer: nil,
		done:      make(chan WaitResponse, 1),
		cleanup:   func() {},
	}

	d.addWaiter(req.SessionID, w)
	defer d.removeWaiter(req.SessionID, w)

	if req.Timeout > 0 {
		timer := time.NewTimer(time.Duration(req.Timeout) * time.Millisecond)
		defer timer.Stop()

		select {
		case resp := <-w.done:
			_ = c.Control().WriteFrame(okFrame(resp))
		case <-timer.C:
			_ = c.Control().WriteFrame(okFrame(WaitResponse{
				SessionID: req.SessionID,
				Mode:      "exit",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  true,
			}))
		}
	} else {
		resp := <-w.done
		_ = c.Control().WriteFrame(okFrame(resp))
	}
}

func (d *Daemon) waitUntilIdle(c ConnectedClient, req WaitRequest) {
	idleDuration := time.Duration(req.IdleFor) * time.Millisecond

	idleTimer := time.NewTimer(idleDuration)
	defer idleTimer.Stop()

	w := &waiter{
		mode:      "idle",
		pattern:   nil,
		idleFor:   idleDuration,
		idleTimer: idleTimer,
		done:      make(chan WaitResponse, 1),
		cleanup:   func() {},
	}

	d.addWaiter(req.SessionID, w)
	defer d.removeWaiter(req.SessionID, w)

	if req.Timeout > 0 {
		timeoutTimer := time.NewTimer(time.Duration(req.Timeout) * time.Millisecond)
		defer timeoutTimer.Stop()

		select {
		case <-idleTimer.C:
			_ = c.Control().WriteFrame(okFrame(WaitResponse{
				SessionID: req.SessionID,
				Mode:      "idle",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  false,
			}))
		case resp := <-w.done:
			_ = c.Control().WriteFrame(okFrame(resp))
		case <-timeoutTimer.C:
			_ = c.Control().WriteFrame(okFrame(WaitResponse{
				SessionID: req.SessionID,
				Mode:      "idle",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  true,
			}))
		}
	} else {
		select {
		case <-idleTimer.C:
			_ = c.Control().WriteFrame(okFrame(WaitResponse{
				SessionID: req.SessionID,
				Mode:      "idle",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  false,
			}))
		case resp := <-w.done:
			_ = c.Control().WriteFrame(okFrame(resp))
		}
	}
}

func (d *Daemon) waitUntilMatch(c ConnectedClient, req WaitRequest) {
	w := &waiter{
		mode:      "match",
		pattern:   []byte(req.Pattern),
		idleFor:   0,
		idleTimer: nil,
		done:      make(chan WaitResponse, 1),
		cleanup:   func() {},
	}

	d.addWaiter(req.SessionID, w)
	defer d.removeWaiter(req.SessionID, w)

	if req.Timeout > 0 {
		timer := time.NewTimer(time.Duration(req.Timeout) * time.Millisecond)
		defer timer.Stop()

		select {
		case resp := <-w.done:
			_ = c.Control().WriteFrame(okFrame(resp))
		case <-timer.C:
			_ = c.Control().WriteFrame(okFrame(WaitResponse{
				SessionID: req.SessionID,
				Mode:      "match",
				ExitCode:  nil,
				Matched:   false,
				TimedOut:  true,
			}))
		}
	} else {
		resp := <-w.done
		_ = c.Control().WriteFrame(okFrame(resp))
	}
}

// persistSessionCreate writes initial metadata and creates a scrollback writer
// when cold restore is enabled.
func (d *Daemon) persistSessionCreate(sessionID string, req CreateRequest) {
	if !d.coldRestore || d.dataDir == "" {
		return
	}

	sessionDir, err := history.EnsureSessionDir(d.dataDir, sessionID)
	if err != nil {
		return
	}

	meta := history.Metadata{
		SessionID: sessionID,
		Shell:     req.Shell,
		Cwd:       req.Cwd,
		Cols:      req.Cols,
		Rows:      req.Rows,
		StartedAt: time.Now(),
		EndedAt:   nil,
		ExitCode:  nil,
	}

	if err := history.WriteMetadata(sessionDir, meta); err != nil {
		return
	}

	w, err := history.NewWriter(filepath.Join(sessionDir, "scrollback.bin"), d.maxScrollbackSize)
	if err != nil {
		return
	}

	d.mu.Lock()
	d.scrollbackWriters[sessionID] = w
	d.mu.Unlock()
}

// persistSessionExit updates metadata with exit info and closes the scrollback
// writer when cold restore is enabled.
func (d *Daemon) persistSessionExit(sessionID string, exitCode int) {
	if !d.coldRestore || d.dataDir == "" {
		return
	}

	d.mu.Lock()
	w, ok := d.scrollbackWriters[sessionID]
	if ok {
		delete(d.scrollbackWriters, sessionID)
	}
	d.mu.Unlock()

	if w != nil {
		_ = w.Close()
	}

	sessionDir := filepath.Join(d.dataDir, sessionID)
	_ = history.UpdateMetadataExit(sessionDir, time.Now(), exitCode)
}

// persistOutput writes output data to the scrollback writer for the given
// session when cold restore is enabled.
func (d *Daemon) persistOutput(sessionID string, data []byte) {
	d.mu.RLock()
	w := d.scrollbackWriters[sessionID]
	d.mu.RUnlock()

	if w != nil {
		_, _ = w.Write(data)
	}
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
