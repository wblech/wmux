package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/platform/pty"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
)

// --- test adapters: wrap real transport/session behind the daemon interfaces ---

// testServerAdapter wraps *transport.Server to implement TransportServer.
type testServerAdapter struct {
	srv *transport.Server
}

func (a *testServerAdapter) OnClient(fn func(ConnectedClient)) {
	a.srv.OnClient(func(c *transport.Client) {
		fn(&testClientAdapter{c: c})
	})
}

func (a *testServerAdapter) Serve(ctx context.Context) error {
	return a.srv.Serve(ctx) //nolint:wrapcheck
}

func (a *testServerAdapter) BroadcastTo(clientID string, f protocol.Frame) error {
	return a.srv.BroadcastTo(clientID, f) //nolint:wrapcheck
}

// testClientAdapter wraps *transport.Client to implement ConnectedClient.
type testClientAdapter struct {
	c *transport.Client
}

func (a *testClientAdapter) ClientID() string {
	return a.c.ID
}

func (a *testClientAdapter) Control() ControlConn {
	return a.c.Control
}

// testSessionAdapter wraps *session.Service to implement SessionManager.
type testSessionAdapter struct {
	svc *session.Service
}

func (a *testSessionAdapter) Create(id string, opts SessionCreateOptions) (SessionInfo, error) {
	sess, err := a.svc.Create(id, session.CreateOptions{
		Shell:         opts.Shell,
		Args:          opts.Args,
		Cols:          opts.Cols,
		Rows:          opts.Rows,
		Cwd:           opts.Cwd,
		Env:           opts.Env,
		HighWatermark: 0,
		LowWatermark:  0,
		BatchInterval: 0,
		HistoryWriter: nil,
	})
	if err != nil {
		return SessionInfo{}, err //nolint:wrapcheck
	}

	return SessionInfo{
		ID:    sess.ID,
		State: sess.State.String(),
		Pid:   sess.Pid,
		Cols:  sess.Cols,
		Rows:  sess.Rows,
		Shell: sess.Shell,
	}, nil
}

func (a *testSessionAdapter) Get(id string) (SessionInfo, error) {
	sess, err := a.svc.Get(id)
	if err != nil {
		return SessionInfo{}, err //nolint:wrapcheck
	}

	return SessionInfo{
		ID:    sess.ID,
		State: sess.State.String(),
		Pid:   sess.Pid,
		Cols:  sess.Cols,
		Rows:  sess.Rows,
		Shell: sess.Shell,
	}, nil
}

func (a *testSessionAdapter) List() []SessionInfo {
	sessions := a.svc.List()
	infos := make([]SessionInfo, 0, len(sessions))

	for _, sess := range sessions {
		infos = append(infos, SessionInfo{
			ID:    sess.ID,
			State: sess.State.String(),
			Pid:   sess.Pid,
			Cols:  sess.Cols,
			Rows:  sess.Rows,
			Shell: sess.Shell,
		})
	}

	return infos
}

func (a *testSessionAdapter) Kill(id string) error {
	return a.svc.Kill(id) //nolint:wrapcheck
}

func (a *testSessionAdapter) Resize(id string, cols, rows int) error {
	return a.svc.Resize(id, cols, rows) //nolint:wrapcheck
}

func (a *testSessionAdapter) WriteInput(id string, data []byte) error {
	return a.svc.WriteInput(id, data) //nolint:wrapcheck
}

func (a *testSessionAdapter) ReadOutput(id string) ([]byte, error) {
	return a.svc.ReadOutput(id) //nolint:wrapcheck
}

func (a *testSessionAdapter) Attach(id string) error {
	return a.svc.Attach(id) //nolint:wrapcheck
}

func (a *testSessionAdapter) Detach(id string) error {
	return a.svc.Detach(id) //nolint:wrapcheck
}

func (a *testSessionAdapter) Snapshot(id string) (SnapshotData, error) {
	snap, err := a.svc.Snapshot(id)
	if err != nil {
		return SnapshotData{}, err //nolint:wrapcheck
	}
	return SnapshotData{Scrollback: snap.Scrollback, Viewport: snap.Viewport}, nil
}

func (a *testSessionAdapter) LastActivity(id string) (time.Time, error) {
	return a.svc.LastActivity(id) //nolint:wrapcheck
}

func (a *testSessionAdapter) MetaSet(id, key, value string) error {
	return a.svc.MetaSet(id, key, value) //nolint:wrapcheck
}

func (a *testSessionAdapter) MetaGet(id, key string) (string, error) {
	return a.svc.MetaGet(id, key) //nolint:wrapcheck
}

func (a *testSessionAdapter) MetaGetAll(id string) (map[string]string, error) {
	return a.svc.MetaGetAll(id) //nolint:wrapcheck
}

func (a *testSessionAdapter) OnExit(fn func(id string, exitCode int)) {
	a.svc.OnExit(fn)
}

// --- test helpers ---

// testDaemon creates a Daemon with a temp Unix socket. It returns the daemon,
// the auth token, and the socket path. Cleanup is registered on t.
func testDaemon(t *testing.T, opts ...Option) (*Daemon, []byte, string) {
	t.Helper()

	dir, err := os.MkdirTemp("", "wmux-daemon")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sock := filepath.Join(dir, "d.sock")

	ln, err := ipc.Listen(sock)
	require.NoError(t, err)

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := transport.NewServer(ln, token)
	spawner := &pty.UnixSpawner{}
	svc := session.NewService(spawner)
	d := NewDaemon(&testServerAdapter{srv: srv}, &testSessionAdapter{svc: svc}, opts...)

	return d, token, sock
}

// startDaemon launches d.Start in a goroutine and returns a cancel func.
func startDaemon(t *testing.T, d *Daemon) context.CancelFunc {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	go func() { _ = d.Start(ctx) }()

	time.Sleep(20 * time.Millisecond)

	return cancel
}

// dialControl authenticates a control channel and returns the protocol.Conn
// and the server-assigned client ID.
func dialControl(t *testing.T, sockPath string, token []byte) (*protocol.Conn, string) {
	t.Helper()

	raw, err := net.Dial("unix", sockPath)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)

	payload := make([]byte, 0, 1+auth.TokenSize)
	payload = append(payload, byte(transport.ChannelControl))
	payload = append(payload, token...)

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)
	require.Equal(t, protocol.MsgOK, resp.Type)

	return conn, string(resp.Payload)
}

// dialStream authenticates a stream channel for the given clientID and returns
// the protocol.Conn.
func dialStream(t *testing.T, sockPath string, token []byte, clientID string) *protocol.Conn {
	t.Helper()

	raw, err := net.Dial("unix", sockPath)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)

	payload := make([]byte, 0, 1+auth.TokenSize+1+len(clientID))
	payload = append(payload, byte(transport.ChannelStream))
	payload = append(payload, token...)
	payload = append(payload, byte(len(clientID)))
	payload = append(payload, []byte(clientID)...)

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)
	require.Equal(t, protocol.MsgOK, resp.Type)

	return conn
}

// sendControl sends a control frame with a JSON-encoded payload and reads the
// response frame.
func sendControl(t *testing.T, conn *protocol.Conn, msgType protocol.MessageType, payload any) protocol.Frame {
	t.Helper()

	var data []byte

	if payload != nil {
		var err error
		data, err = json.Marshal(payload)
		require.NoError(t, err)
	}

	err := conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Payload: data,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	return resp
}

// TestDaemon_CreateSession verifies that sending MsgCreate returns MsgOK with
// a SessionResponse in the payload.
func TestDaemon_CreateSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "test-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})

	require.Equal(t, protocol.MsgOK, resp.Type)

	var sr SessionResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &sr))
	assert.Equal(t, "test-sess", sr.ID)
	assert.Equal(t, "alive", sr.State)
}

// TestDaemon_ListSessions creates one session and verifies MsgList returns it.
func TestDaemon_ListSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "list-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	listResp := sendControl(t, ctrl, protocol.MsgList, nil)
	require.Equal(t, protocol.MsgOK, listResp.Type)

	var sessions []SessionResponse
	require.NoError(t, json.Unmarshal(listResp.Payload, &sessions))
	assert.Len(t, sessions, 1)
	assert.Equal(t, "list-sess", sessions[0].ID)
}

// TestDaemon_KillSession creates a session, kills it, and verifies MsgList
// returns an empty list.
func TestDaemon_KillSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "kill-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	killResp := sendControl(t, ctrl, protocol.MsgKill, SessionIDRequest{SessionID: "kill-sess"})
	require.Equal(t, protocol.MsgOK, killResp.Type)

	listResp := sendControl(t, ctrl, protocol.MsgList, nil)
	require.Equal(t, protocol.MsgOK, listResp.Type)

	var sessions []SessionResponse
	require.NoError(t, json.Unmarshal(listResp.Payload, &sessions))
	assert.Empty(t, sessions)
}

// TestDaemon_ResizeSession creates a session, resizes it, and verifies the new
// dimensions are reflected by MsgInfo.
func TestDaemon_ResizeSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "resize-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	resizeResp := sendControl(t, ctrl, protocol.MsgResize, ResizeRequest{
		SessionID: "resize-sess",
		Cols:      120,
		Rows:      40,
	})
	require.Equal(t, protocol.MsgOK, resizeResp.Type)

	infoResp := sendControl(t, ctrl, protocol.MsgInfo, SessionIDRequest{SessionID: "resize-sess"})
	require.Equal(t, protocol.MsgOK, infoResp.Type)

	var sr SessionResponse
	require.NoError(t, json.Unmarshal(infoResp.Payload, &sr))
	assert.Equal(t, 120, sr.Cols)
	assert.Equal(t, 40, sr.Rows)
}

// TestDaemon_AttachAndReceiveOutput creates a session, attaches to it, writes
// input, and checks that MsgData arrives on the stream channel.
func TestDaemon_AttachAndReceiveOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, clientID := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	stream := dialStream(t, sock, token, clientID)
	defer stream.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "attach-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	attachResp := sendControl(t, ctrl, protocol.MsgAttach, SessionIDRequest{SessionID: "attach-sess"})
	require.Equal(t, protocol.MsgOK, attachResp.Type)

	// Send input that will produce output (echo via the shell).
	inputPayload := EncodeInputPayload("attach-sess", []byte("echo hello\n"))
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgInput,
		Payload: inputPayload,
	})
	require.NoError(t, err)

	// Read the MsgOK acknowledgement for the input frame.
	inputAck, err := ctrl.ReadFrame()
	require.NoError(t, err)
	require.Equal(t, protocol.MsgOK, inputAck.Type)

	// Wait for output data on the stream channel. The broadcaster polls at 16ms
	// and the shell needs a moment to produce output.
	stream.Raw().SetReadDeadline(time.Now().Add(3 * time.Second)) //nolint:errcheck

	dataFrame, err := stream.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgData, dataFrame.Type)

	sessID, data, err := DecodeDataPayload(dataFrame.Payload)
	require.NoError(t, err)
	assert.Equal(t, "attach-sess", sessID)
	assert.NotEmpty(t, data)
}

// TestDaemon_DetachStopsOutput creates a session, attaches, then detaches and
// verifies the detach returns MsgOK.
func TestDaemon_DetachStopsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "detach-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	attachResp := sendControl(t, ctrl, protocol.MsgAttach, SessionIDRequest{SessionID: "detach-sess"})
	require.Equal(t, protocol.MsgOK, attachResp.Type)

	detachResp := sendControl(t, ctrl, protocol.MsgDetach, SessionIDRequest{SessionID: "detach-sess"})
	assert.Equal(t, protocol.MsgOK, detachResp.Type)
}

// TestDaemon_Shutdown sends MsgShutdown and verifies MsgOK is received and the
// socket becomes unreachable shortly after.
func TestDaemon_Shutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgShutdown, nil)
	assert.Equal(t, protocol.MsgOK, resp.Type)

	// After the shutdown delay the socket should be unreachable.
	time.Sleep(200 * time.Millisecond)

	_, err := net.Dial("unix", sock)
	assert.Error(t, err, "expected socket to be unreachable after shutdown")
}

// TestDaemon_PIDFileWritten verifies that Start writes a PID file with the
// current process PID.
func TestDaemon_PIDFileWritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir, err := os.MkdirTemp("", "wmux-pid")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	pidPath := filepath.Join(dir, "wmux.pid")

	d, _, _ := testDaemon(t, WithPIDFilePath(pidPath))
	cancel := startDaemon(t, d)
	defer cancel()

	// Give Start a moment to write the PID file.
	time.Sleep(30 * time.Millisecond)

	info, err := ReadPIDFile(pidPath)
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), info.PID)
}

// TestDaemon_InfoNotFound verifies that MsgInfo for a nonexistent session
// returns MsgError.
func TestDaemon_InfoNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgInfo, SessionIDRequest{SessionID: "no-such-session"})
	assert.Equal(t, protocol.MsgError, resp.Type)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &errResp))
	assert.NotEmpty(t, errResp.Error)
}

// TestDaemon_InputToSession creates a session, sends MsgInput, and expects
// MsgOK in return.
func TestDaemon_InputToSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "input-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	inputPayload := EncodeInputPayload("input-sess", []byte("echo test\n"))
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgInput,
		Payload: inputPayload,
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgOK, resp.Type)
}

// TestDaemon_WithVersion verifies that WithVersion sets the version field.
func TestDaemon_WithVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, _, _ := testDaemon(t, WithVersion("1.2.3"))
	d.mu.RLock()
	v := d.version
	d.mu.RUnlock()
	assert.Equal(t, "1.2.3", v)
}

// TestDaemon_WithDataDir verifies that WithDataDir sets the dataDir field.
func TestDaemon_WithDataDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, _, _ := testDaemon(t, WithDataDir("/tmp/data"))
	d.mu.RLock()
	dir := d.dataDir
	d.mu.RUnlock()
	assert.Equal(t, "/tmp/data", dir)
}

// TestDaemon_CreateInvalidPayload verifies that MsgCreate with invalid JSON
// returns MsgError.
func TestDaemon_CreateInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgCreate,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_KillInvalidPayload verifies that MsgKill with bad JSON returns
// MsgError.
func TestDaemon_KillInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgKill,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_ResizeInvalidPayload verifies that MsgResize with bad JSON
// returns MsgError.
func TestDaemon_ResizeInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgResize,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_InputInvalidPayload verifies that MsgInput with a malformed binary
// payload returns MsgError.
func TestDaemon_InputInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Empty payload triggers ErrPayloadTooShort.
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgInput,
		Payload: []byte{},
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_AttachInvalidPayload verifies that MsgAttach with bad JSON returns
// MsgError.
func TestDaemon_AttachInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAttach,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_DetachInvalidPayload verifies that MsgDetach with bad JSON returns
// MsgError.
func TestDaemon_DetachInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgDetach,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_InfoInvalidPayload verifies that MsgInfo with bad JSON returns
// MsgError.
func TestDaemon_InfoInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgInfo,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_KillNotFound verifies that MsgKill for a nonexistent session
// returns MsgError.
func TestDaemon_KillNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgKill, SessionIDRequest{SessionID: "no-such"})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_ResizeNotFound verifies that MsgResize for a nonexistent session
// returns MsgError.
func TestDaemon_ResizeNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgResize, ResizeRequest{SessionID: "no-such", Cols: 80, Rows: 24})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_InputNotFound verifies that MsgInput to a nonexistent session
// returns MsgError.
func TestDaemon_InputNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	inputPayload := EncodeInputPayload("no-such-sess", []byte("data"))
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgInput,
		Payload: inputPayload,
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_UnknownMessage verifies that sending an unknown message type
// returns MsgError.
func TestDaemon_UnknownMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgHeartbeat, // not handled by daemon
		Payload: nil,
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_AttachDetachUpdatesSessionState verifies that attach/detach
// messages transition the underlying session state correctly.
func TestDaemon_AttachDetachUpdatesSessionState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	// Build daemon with direct access to the underlying session service.
	dir, err := os.MkdirTemp("", "wmux-daemon-ad")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sock := filepath.Join(dir, "d.sock")
	ln, err := ipc.Listen(sock)
	require.NoError(t, err)

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := transport.NewServer(ln, token)
	spawner := &pty.UnixSpawner{}
	sessSvc := session.NewService(spawner)
	d := NewDaemon(&testServerAdapter{srv: srv}, &testSessionAdapter{svc: sessSvc})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create session.
	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "state-1",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Session starts alive.
	sess, err := sessSvc.Get("state-1")
	require.NoError(t, err)
	assert.Equal(t, session.StateAlive, sess.State)

	// Attach transitions session to alive (from alive it stays alive).
	attachResp := sendControl(t, ctrl, protocol.MsgAttach, SessionIDRequest{SessionID: "state-1"})
	require.Equal(t, protocol.MsgOK, attachResp.Type)

	// Detach — last client leaves, so session should be detached.
	detachResp := sendControl(t, ctrl, protocol.MsgDetach, SessionIDRequest{SessionID: "state-1"})
	require.Equal(t, protocol.MsgOK, detachResp.Type)

	time.Sleep(50 * time.Millisecond)

	sess, err = sessSvc.Get("state-1")
	require.NoError(t, err)
	assert.Equal(t, session.StateDetached, sess.State)
}

// testEventBus is a simple event recorder for tests.
type testEventBus struct {
	mu     sync.Mutex
	events []event.Event
	bus    *event.Bus
}

func newTestEventBus() *testEventBus {
	return &testEventBus{
		mu:     sync.Mutex{},
		events: nil,
		bus:    event.NewBus(),
	}
}

func (b *testEventBus) Publish(e event.Event) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
	b.bus.Publish(e)
}

func (b *testEventBus) Subscribe() *event.Subscription {
	return b.bus.Subscribe()
}

func (b *testEventBus) Events() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]event.Event, len(b.events))
	copy(out, b.events)
	return out
}

func TestDaemon_EmitsSessionCreatedEvent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	eb := newTestEventBus()
	d, token, sock := testDaemon(t, WithEventBus(eb))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "evt-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	time.Sleep(50 * time.Millisecond)

	events := eb.Events()
	require.NotEmpty(t, events)
	assert.Equal(t, event.SessionCreated, events[0].Type)
	assert.Equal(t, "evt-sess", events[0].SessionID)
}

func TestDaemon_EmitsAttachDetachEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	eb := newTestEventBus()
	d, token, sock := testDaemon(t, WithEventBus(eb))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "ad-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})

	sendControl(t, ctrl, protocol.MsgAttach, SessionIDRequest{SessionID: "ad-sess"})
	sendControl(t, ctrl, protocol.MsgDetach, SessionIDRequest{SessionID: "ad-sess"})

	time.Sleep(50 * time.Millisecond)

	events := eb.Events()
	types := make([]event.Type, 0, len(events))
	for _, e := range events {
		types = append(types, e.Type)
	}

	assert.Contains(t, types, event.SessionCreated)
	assert.Contains(t, types, event.SessionAttached)
	assert.Contains(t, types, event.SessionDetached)
}

func TestDaemon_Status(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t, WithVersion("0.1.0"))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "status-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})

	resp := sendControl(t, ctrl, protocol.MsgStatus, nil)
	require.Equal(t, protocol.MsgOK, resp.Type)

	var sr StatusResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &sr))
	assert.Equal(t, "0.1.0", sr.Version)
	assert.Equal(t, 1, sr.SessionCount)
	assert.NotEmpty(t, sr.Uptime)
}

func TestDaemon_EventSubscribeReceivesEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	eb := newTestEventBus()
	d, token, sock := testDaemon(t, WithEventBus(eb))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Subscribe to events.
	resp := sendControl(t, ctrl, protocol.MsgEvent, nil)
	require.Equal(t, protocol.MsgOK, resp.Type)

	// Create a session to trigger an event.
	ctrl2, _ := dialControl(t, sock, token)
	defer ctrl2.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl2, protocol.MsgCreate, CreateRequest{
		ID:    "sub-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Read the event from the subscription.
	ctrl.Raw().SetReadDeadline(time.Now().Add(3 * time.Second)) //nolint:errcheck
	evtFrame, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgEvent, evtFrame.Type)

	var evt struct {
		SessionID string `json:"session_id"`
	}
	require.NoError(t, json.Unmarshal(evtFrame.Payload, &evt))
	assert.Equal(t, "sub-sess", evt.SessionID)
}

func TestDaemon_EventSubscribeNoEventBus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	// No WithEventBus — eventBus is nil.
	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgEvent, nil)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_EventSubscribeInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	eb := newTestEventBus()
	d, token, sock := testDaemon(t, WithEventBus(eb))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgEvent,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_AttachReturnsSessionInfo verifies that MsgAttach returns session
// metadata in the AttachResponse payload.
func TestDaemon_AttachReturnsSessionInfo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "snap-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	attachResp := sendControl(t, ctrl, protocol.MsgAttach, SessionIDRequest{SessionID: "snap-sess"})
	require.Equal(t, protocol.MsgOK, attachResp.Type)
	require.NotNil(t, attachResp.Payload)

	var attachData AttachResponse
	err2 := json.Unmarshal(attachResp.Payload, &attachData)
	require.NoError(t, err2)
	assert.Equal(t, "snap-sess", attachData.ID)
	assert.NotEmpty(t, attachData.State)
	assert.Equal(t, 80, attachData.Cols)
	assert.Equal(t, 24, attachData.Rows)
}

func TestDaemon_MetaSetAndGet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "meta-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Set metadata.
	setResp := sendControl(t, ctrl, protocol.MsgMetaSet, MetaSetRequest{
		SessionID: "meta-sess",
		Key:       "app",
		Value:     "watchtower",
	})
	require.Equal(t, protocol.MsgOK, setResp.Type)

	// Get single key.
	getResp := sendControl(t, ctrl, protocol.MsgMetaGet, MetaGetRequest{
		SessionID: "meta-sess",
		Key:       "app",
	})
	require.Equal(t, protocol.MsgOK, getResp.Type)

	var metaResp MetaGetResponse
	require.NoError(t, json.Unmarshal(getResp.Payload, &metaResp))
	assert.Equal(t, "watchtower", metaResp.Value)

	// Get all metadata.
	getAllResp := sendControl(t, ctrl, protocol.MsgMetaGet, MetaGetRequest{
		SessionID: "meta-sess",
		Key:       "",
	})
	require.Equal(t, protocol.MsgOK, getAllResp.Type)

	var allResp MetaGetResponse
	require.NoError(t, json.Unmarshal(getAllResp.Payload, &allResp))
	assert.Equal(t, "watchtower", allResp.Metadata["app"])
}

func TestDaemon_MetaSetNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgMetaSet, MetaSetRequest{
		SessionID: "no-such",
		Key:       "k",
		Value:     "v",
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_MetaSetInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgMetaSet,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_MetaGetInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgMetaGet,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_EnvForward(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := t.TempDir()
	d, token, sock := testDaemon(t, WithDataDir(dir))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "env-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	envResp := sendControl(t, ctrl, protocol.MsgEnvForward, EnvForwardRequest{
		SessionID: "env-sess",
		Env:       map[string]string{"DISPLAY": ":0"},
	})
	require.Equal(t, protocol.MsgOK, envResp.Type)
}

func TestDaemon_EnvForwardNoDataDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t) // no WithDataDir
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	envResp := sendControl(t, ctrl, protocol.MsgEnvForward, EnvForwardRequest{
		SessionID: "whatever",
		Env:       map[string]string{"DISPLAY": ":0"},
	})
	require.Equal(t, protocol.MsgOK, envResp.Type)
}

func TestDaemon_EnvForwardInvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgEnvForward,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

// TestDaemon_ColdRestore_Disabled_NoHistoryWritten verifies that no history
// files are written when cold restore is not enabled.
func TestDaemon_ColdRestore_Disabled_NoHistoryWritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dataDir := t.TempDir()
	d, token, sock := testDaemon(t, WithDataDir(dataDir))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "no-history",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	killResp := sendControl(t, ctrl, protocol.MsgKill, SessionIDRequest{SessionID: "no-history"})
	require.Equal(t, protocol.MsgOK, killResp.Type)

	time.Sleep(50 * time.Millisecond)

	// No session directory should exist.
	_, err := os.Stat(filepath.Join(dataDir, "no-history"))
	assert.True(t, os.IsNotExist(err))
}

// TestDaemon_ColdRestore_Enabled_WritesMetadata verifies that metadata is
// written on session create when cold restore is enabled.
func TestDaemon_ColdRestore_Enabled_WritesMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dataDir := t.TempDir()
	d, token, sock := testDaemon(t, WithDataDir(dataDir), WithColdRestore(true))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "cold-meta",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	sessionDir := filepath.Join(dataDir, "cold-meta")
	_, err := os.Stat(filepath.Join(sessionDir, "meta.json"))
	require.NoError(t, err, "meta.json should exist after create")

	_, err = os.Stat(filepath.Join(sessionDir, "scrollback.bin"))
	require.NoError(t, err, "scrollback.bin should exist after create")
}

// TestDaemon_ColdRestore_Enabled_ExitUpdatesMetadata verifies that metadata is
// updated with exit info when a session exits and cold restore is enabled.
func TestDaemon_ColdRestore_Enabled_ExitUpdatesMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dataDir := t.TempDir()
	eb := newTestEventBus()
	d, token, sock := testDaemon(t, WithDataDir(dataDir), WithColdRestore(true), WithEventBus(eb))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "cold-exit",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	// Kill the session and wait for the SessionExited event.
	killResp := sendControl(t, ctrl, protocol.MsgKill, SessionIDRequest{SessionID: "cold-exit"})
	require.Equal(t, protocol.MsgOK, killResp.Type)

	sessionDir := filepath.Join(dataDir, "cold-exit")
	require.Eventually(t, func() bool {
		for _, evt := range eb.Events() {
			if evt.Type == event.SessionExited && evt.SessionID == "cold-exit" {
				return true
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond, "should receive SessionExited event")

	metaData, err := os.ReadFile(filepath.Join(sessionDir, "meta.json"))
	require.NoError(t, err)
	assert.Contains(t, string(metaData), "ended_at")
}

// TestDaemon_ColdRestore_Enabled_ScrollbackPersisted verifies that output data
// is written to scrollback.bin when cold restore is enabled.
func TestDaemon_ColdRestore_Enabled_ScrollbackPersisted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dataDir := t.TempDir()
	d, token, sock := testDaemon(t, WithDataDir(dataDir), WithColdRestore(true))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, clientID := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	stream := dialStream(t, sock, token, clientID)
	defer stream.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "cold-scroll",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	attachResp := sendControl(t, ctrl, protocol.MsgAttach, SessionIDRequest{SessionID: "cold-scroll"})
	require.Equal(t, protocol.MsgOK, attachResp.Type)

	inputPayload := EncodeInputPayload("cold-scroll", []byte("echo cold-restore-test\n"))
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgInput,
		Payload: inputPayload,
	})
	require.NoError(t, err)

	inputAck, err := ctrl.ReadFrame()
	require.NoError(t, err)
	require.Equal(t, protocol.MsgOK, inputAck.Type)

	// Wait for output to be broadcast and persisted to scrollback.
	sessionDir := filepath.Join(dataDir, "cold-scroll")
	require.Eventually(t, func() bool {
		info, err := os.Stat(filepath.Join(sessionDir, "scrollback.bin"))
		return err == nil && info.Size() > 0
	}, 3*time.Second, 50*time.Millisecond, "scrollback.bin should have data")
}

func TestDaemon_ColdRestore_MaxScrollbackSize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dataDir := t.TempDir()
	eb := event.NewBus()
	defer eb.Close()

	d, token, sock := testDaemon(t,
		WithDataDir(dataDir),
		WithColdRestore(true),
		WithMaxScrollbackSize(1024),
		WithEventBus(eb),
	)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "scroll-test",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Verify the scrollback writer was created.
	sessionDir := filepath.Join(dataDir, "scroll-test")
	scrollbackPath := filepath.Join(sessionDir, "scrollback.bin")

	require.Eventually(t, func() bool {
		_, err := os.Stat(scrollbackPath)
		return err == nil
	}, 2*time.Second, 50*time.Millisecond)
}

// --- Phase 3: exec tests ---

func TestDaemon_Exec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "exec-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	resp := sendControl(t, ctrl, protocol.MsgExec, ExecRequest{
		SessionID: "exec-sess",
		Input:     "echo hello",
		Newline:   true,
	})
	assert.Equal(t, protocol.MsgOK, resp.Type)
}

func TestDaemon_Exec_NoNewline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "exec-nonl",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	resp := sendControl(t, ctrl, protocol.MsgExec, ExecRequest{
		SessionID: "exec-nonl",
		Input:     "partial",
		Newline:   false,
	})
	assert.Equal(t, protocol.MsgOK, resp.Type)
}

func TestDaemon_Exec_NotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgExec, ExecRequest{
		SessionID: "nonexistent",
		Input:     "ls",
		Newline:   true,
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_Exec_InvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgExec,
		Payload: []byte("not json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_ExecSync_ByIDs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	for _, id := range []string{"sync-s1", "sync-s2"} {
		resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
			ID:    id,
			Shell: "/bin/sh",
			Args:  nil,
			Cols:  80,
			Rows:  24,
			Cwd:   "",
			Env:   nil,
		})
		require.Equal(t, protocol.MsgOK, resp.Type)
	}

	resp := sendControl(t, ctrl, protocol.MsgExecSync, ExecSyncRequest{
		SessionIDs: []string{"sync-s1", "sync-s2"},
		Prefix:     "",
		Input:      "echo sync",
		Newline:    true,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	var syncResp ExecSyncResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &syncResp))
	assert.Len(t, syncResp.Results, 2)
	assert.True(t, syncResp.Results[0].OK)
	assert.True(t, syncResp.Results[1].OK)
}

func TestDaemon_ExecSync_PartialFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "partial-s1",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	syncResp := sendControl(t, ctrl, protocol.MsgExecSync, ExecSyncRequest{
		SessionIDs: []string{"partial-s1", "nonexistent"},
		Prefix:     "",
		Input:      "cmd",
		Newline:    true,
	})
	require.Equal(t, protocol.MsgOK, syncResp.Type)

	var result ExecSyncResponse
	require.NoError(t, json.Unmarshal(syncResp.Payload, &result))
	assert.Len(t, result.Results, 2)
	assert.True(t, result.Results[0].OK)
	assert.False(t, result.Results[1].OK)
	assert.Contains(t, result.Results[1].Error, "not found")
}

func TestDaemon_ExecSync_ByPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	for _, id := range []string{"proj-a/s1", "proj-a/s2", "proj-b/s3"} {
		resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
			ID:    id,
			Shell: "/bin/sh",
			Args:  nil,
			Cols:  80,
			Rows:  24,
			Cwd:   "",
			Env:   nil,
		})
		require.Equal(t, protocol.MsgOK, resp.Type)
	}

	resp := sendControl(t, ctrl, protocol.MsgExecSync, ExecSyncRequest{
		SessionIDs: nil,
		Prefix:     "proj-a",
		Input:      "ls",
		Newline:    true,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	var syncResp ExecSyncResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &syncResp))
	assert.Len(t, syncResp.Results, 2)
	for _, r := range syncResp.Results {
		assert.True(t, r.OK)
	}
}

func TestDaemon_ExecSync_BothIDsAndPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgExecSync, ExecSyncRequest{
		SessionIDs: []string{"s1"},
		Prefix:     "proj-a",
		Input:      "cmd",
		Newline:    false,
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_ExecSync_NoMatchingSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgExecSync, ExecSyncRequest{
		SessionIDs: nil,
		Prefix:     "nonexistent",
		Input:      "cmd",
		Newline:    false,
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_ExecSync_InvalidPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgExecSync,
		Payload: []byte("not json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}
