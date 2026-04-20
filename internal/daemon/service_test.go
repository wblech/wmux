package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/history"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/platform/pty"
	"github.com/wblech/wmux/internal/platform/recording"
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
	return SnapshotData{Replay: snap.Replay}, nil
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

func (a *testSessionAdapter) UpdateEmulatorScrollback(id string, scrollbackLines int) error {
	return a.svc.UpdateEmulatorScrollback(id, scrollbackLines) //nolint:wrapcheck
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

// testDaemonWithSessionOpts is like testDaemon but allows passing session.Option
// to the underlying session.Service. This enables tests that wire AddonManager.
func testDaemonWithSessionOpts(t *testing.T, sessionOpts []session.Option, daemonOpts ...Option) (*Daemon, []byte, string) { //nolint:unused // helper prepared for upcoming integration tests
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
	svc := session.NewService(spawner, sessionOpts...)
	d := NewDaemon(&testServerAdapter{srv: srv}, &testSessionAdapter{svc: svc}, daemonOpts...)

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

// TestDaemon_AttachSnapshotWithData uses a spy SessionManager to verify that
// handleAttach correctly populates the Snapshot field when the SessionManager
// returns non-empty snapshot data.
func TestDaemon_AttachSnapshotWithData(t *testing.T) {
	sm := &snapshotSpySessionManager{
		noopSessionManager: noopSessionManager{},
		snapshotData: SnapshotData{
			Replay: []byte("\x1b[2J\x1b[Hline1\r\nline2\r\ncurrent-view"),
		},
	}

	bus := &spyEventBus{mu: sync.Mutex{}, events: nil}
	d := newTestDaemonUnit(sm, bus, map[string]map[string]struct{}{})

	// Simulate a client connection via a mock control conn.
	mockCtrl := &mockControlConn{
		frames:   nil,
		writeMu:  sync.Mutex{},
		written:  nil,
		writeErr: nil,
	}
	mockClient := &mockConnectedClient{id: "test-client", ctrl: mockCtrl}

	reqPayload, _ := json.Marshal(SessionIDRequest{SessionID: "snap-test"})
	d.handleAttach(mockClient, protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAttach,
		Payload: reqPayload,
	})

	require.Len(t, mockCtrl.written, 1, "should write exactly one response frame")

	resp := mockCtrl.written[0]
	require.Equal(t, protocol.MsgOK, resp.Type)

	var attachData AttachResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &attachData))
	require.NotNil(t, attachData.Snapshot, "Snapshot should be non-nil when SessionManager returns data")
	assert.Equal(t, []byte("\x1b[2J\x1b[Hline1\r\nline2\r\ncurrent-view"), attachData.Snapshot.Replay)
}

// TestDaemon_AttachSnapshotPopulated_Regression is a regression guard ensuring
// handleAttach includes snapshot data when the SessionManager returns it.
// This guards against the wiring bug where Serve() forgot WithAddonManager.
func TestDaemon_AttachSnapshotPopulated_Regression(t *testing.T) {
	sm := &snapshotSpySessionManager{
		noopSessionManager: noopSessionManager{},
		snapshotData: SnapshotData{
			Replay: []byte("\x1b[2J\x1b[Hscroll-data-view-data"),
		},
	}

	bus := &spyEventBus{mu: sync.Mutex{}, events: nil}
	d := newTestDaemonUnit(sm, bus, map[string]map[string]struct{}{})

	mockCtrl := &mockControlConn{
		frames:   nil,
		writeMu:  sync.Mutex{},
		written:  nil,
		writeErr: nil,
	}
	mockClient := &mockConnectedClient{id: "test-client", ctrl: mockCtrl}

	reqPayload, _ := json.Marshal(SessionIDRequest{SessionID: "snap-test"})
	d.handleAttach(mockClient, protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAttach,
		Payload: reqPayload,
	})

	require.Len(t, mockCtrl.written, 1)
	resp := mockCtrl.written[0]
	require.Equal(t, protocol.MsgOK, resp.Type)

	var attachData AttachResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &attachData))

	require.NotNil(t, attachData.Snapshot,
		"REGRESSION: handleAttach must include snapshot when SessionManager returns data")
	assert.Equal(t, []byte("\x1b[2J\x1b[Hscroll-data-view-data"), attachData.Snapshot.Replay)
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

	// Subscribe before kill so we don't miss the event.
	sub := eb.bus.SubscribeTypes(event.SessionExited)
	defer sub.Unsubscribe()

	killResp := sendControl(t, ctrl, protocol.MsgKill, SessionIDRequest{SessionID: "cold-exit"})
	require.Equal(t, protocol.MsgOK, killResp.Type)

	sessionDir := filepath.Join(dataDir, "cold-exit")
	select {
	case evt := <-sub.Events():
		require.Equal(t, event.SessionExited, evt.Type)
		require.Equal(t, "cold-exit", evt.SessionID)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for SessionExited event")
	}

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

func TestDaemon_Wait_UntilExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create a session with a short-lived command.
	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-exit",
		Shell: "/bin/sh",
		Args:  []string{"-c", "sleep 0.1; exit 42"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Send wait request — process should exit quickly with code 42.
	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-exit",
		Mode:      "exit",
		Timeout:   5000,
		IdleFor:   0,
		Pattern:   "",
	})
	require.Equal(t, protocol.MsgOK, waitResp.Type)

	var result WaitResponse
	require.NoError(t, json.Unmarshal(waitResp.Payload, &result))
	assert.Equal(t, "exit", result.Mode)
	assert.Equal(t, "wait-exit", result.SessionID)
	assert.False(t, result.TimedOut)
	require.NotNil(t, result.ExitCode)
	assert.Equal(t, 42, *result.ExitCode)
}

func TestDaemon_Wait_UntilExit_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create a long-running session.
	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-exit-to",
		Shell: "/bin/sh",
		Args:  []string{"-c", "sleep 60"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Wait with a short timeout.
	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-exit-to",
		Mode:      "exit",
		Timeout:   200,
		IdleFor:   0,
		Pattern:   "",
	})
	require.Equal(t, protocol.MsgOK, waitResp.Type)

	var result WaitResponse
	require.NoError(t, json.Unmarshal(waitResp.Payload, &result))
	assert.True(t, result.TimedOut)
	assert.Nil(t, result.ExitCode)
}

func TestDaemon_Wait_InvalidSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "nonexistent",
		Mode:      "exit",
		Timeout:   0,
		IdleFor:   0,
		Pattern:   "",
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_Wait_InvalidMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-invalid-mode",
		Shell: "/bin/sh",
		Args:  []string{"-c", "sleep 60"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-invalid-mode",
		Mode:      "bogus",
		Timeout:   0,
		IdleFor:   0,
		Pattern:   "",
	})
	assert.Equal(t, protocol.MsgError, waitResp.Type)
}

func TestDaemon_Wait_InvalidPayload(t *testing.T) {
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
		Type:    protocol.MsgWait,
		Payload: []byte("not json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_Wait_UntilIdle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create a session that produces output briefly then stops.
	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-idle",
		Shell: "/bin/sh",
		Args:  []string{"-c", "echo hello; sleep 60"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Wait for session to start producing output.
	time.Sleep(100 * time.Millisecond)

	// Wait until idle for 300ms.
	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-idle",
		Mode:      "idle",
		Timeout:   5000,
		IdleFor:   300,
		Pattern:   "",
	})
	require.Equal(t, protocol.MsgOK, waitResp.Type)

	var result WaitResponse
	require.NoError(t, json.Unmarshal(waitResp.Payload, &result))
	assert.Equal(t, "idle", result.Mode)
	assert.False(t, result.TimedOut)
}

func TestDaemon_Wait_UntilIdle_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create a session that keeps producing output.
	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-idle-to",
		Shell: "/bin/sh",
		Args:  []string{"-c", "while true; do echo busy; sleep 0.05; done"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	time.Sleep(50 * time.Millisecond)

	// IdleFor is longer than timeout — should timeout.
	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-idle-to",
		Mode:      "idle",
		Timeout:   300,
		IdleFor:   5000,
		Pattern:   "",
	})
	require.Equal(t, protocol.MsgOK, waitResp.Type)

	var result WaitResponse
	require.NoError(t, json.Unmarshal(waitResp.Payload, &result))
	assert.True(t, result.TimedOut)
}

func TestDaemon_Wait_UntilMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create a session that prints a known marker.
	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-match",
		Shell: "/bin/sh",
		Args:  []string{"-c", "sleep 0.2; echo 'BUILD SUCCESS'; sleep 60"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Wait for the pattern.
	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-match",
		Mode:      "match",
		Timeout:   5000,
		IdleFor:   0,
		Pattern:   "BUILD SUCCESS",
	})
	require.Equal(t, protocol.MsgOK, waitResp.Type)

	var result WaitResponse
	require.NoError(t, json.Unmarshal(waitResp.Payload, &result))
	assert.Equal(t, "match", result.Mode)
	assert.True(t, result.Matched)
	assert.False(t, result.TimedOut)
}

func TestDaemon_Wait_UntilMatch_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "wait-match-to",
		Shell: "/bin/sh",
		Args:  []string{"-c", "echo something else; sleep 60"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	time.Sleep(50 * time.Millisecond)

	waitResp := sendControl(t, ctrl, protocol.MsgWait, WaitRequest{
		SessionID: "wait-match-to",
		Mode:      "match",
		Timeout:   300,
		IdleFor:   0,
		Pattern:   "NEVER APPEARS",
	})
	require.Equal(t, protocol.MsgOK, waitResp.Type)

	var result WaitResponse
	require.NoError(t, json.Unmarshal(waitResp.Payload, &result))
	assert.True(t, result.TimedOut)
	assert.False(t, result.Matched)
}

// --- Record handler tests ---

func TestDaemon_Record_Start(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := t.TempDir()
	d, token, sock := testDaemon(t, WithRecordingDir(dir))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID: "rec-sess", Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	resp := sendControl(t, ctrl, protocol.MsgRecord, RecordRequest{
		SessionID: "rec-sess", Action: "start",
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	var rr RecordResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &rr))
	assert.True(t, rr.Recording)
	assert.Equal(t, "rec-sess", rr.SessionID)
	assert.NotEmpty(t, rr.Path)
}

func TestDaemon_Record_Stop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := t.TempDir()
	d, token, sock := testDaemon(t, WithRecordingDir(dir))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID: "rec-stop", Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	startResp := sendControl(t, ctrl, protocol.MsgRecord, RecordRequest{
		SessionID: "rec-stop", Action: "start",
	})
	require.Equal(t, protocol.MsgOK, startResp.Type)

	stopResp := sendControl(t, ctrl, protocol.MsgRecord, RecordRequest{
		SessionID: "rec-stop", Action: "stop",
	})
	require.Equal(t, protocol.MsgOK, stopResp.Type)

	var rr RecordResponse
	require.NoError(t, json.Unmarshal(stopResp.Payload, &rr))
	assert.False(t, rr.Recording)
}

func TestDaemon_Record_InvalidAction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := t.TempDir()
	d, token, sock := testDaemon(t, WithRecordingDir(dir))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgRecord, RecordRequest{
		SessionID: "s1", Action: "invalid",
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_Record_AlreadyRecording(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir := t.TempDir()
	d, token, sock := testDaemon(t, WithRecordingDir(dir))
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID: "rec-dup", Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	resp1 := sendControl(t, ctrl, protocol.MsgRecord, RecordRequest{
		SessionID: "rec-dup", Action: "start",
	})
	require.Equal(t, protocol.MsgOK, resp1.Type)

	resp2 := sendControl(t, ctrl, protocol.MsgRecord, RecordRequest{
		SessionID: "rec-dup", Action: "start",
	})
	assert.Equal(t, protocol.MsgError, resp2.Type)
}

// --- History handler tests ---

func TestDaemon_History_ANSI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID: "hist-sess", Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// Send some input to generate output.
	sendControl(t, ctrl, protocol.MsgExec, ExecRequest{
		SessionID: "hist-sess", Input: "echo hello-from-history", Newline: true,
	})

	// Give time for output to be generated.
	time.Sleep(200 * time.Millisecond)

	// Send history request manually (multi-frame response).
	histReq, _ := json.Marshal(HistoryRequest{
		SessionID: "hist-sess", Format: "ansi", Lines: 0,
	})
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgHistory,
		Payload: histReq,
	})
	require.NoError(t, err)

	// Read frames until MsgHistoryEnd. NoneEmulator may return empty scrollback,
	// so we only verify the protocol flow completes without error.
	gotEnd := false
	for {
		frame, err := ctrl.ReadFrame()
		require.NoError(t, err)

		if frame.Type == protocol.MsgHistoryEnd {
			gotEnd = true
			break
		}

		if frame.Type == protocol.MsgError {
			t.Fatalf("got error: %s", string(frame.Payload))
		}

		require.Equal(t, protocol.MsgHistory, frame.Type)
	}

	assert.True(t, gotEnd, "should receive MsgHistoryEnd")
}

func TestDaemon_History_Text(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID: "hist-text", Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	sendControl(t, ctrl, protocol.MsgExec, ExecRequest{
		SessionID: "hist-text", Input: "echo histtext", Newline: true,
	})
	time.Sleep(200 * time.Millisecond)

	histReq, _ := json.Marshal(HistoryRequest{
		SessionID: "hist-text", Format: "text", Lines: 0,
	})
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgHistory,
		Payload: histReq,
	})
	require.NoError(t, err)

	var allData []byte
	for {
		frame, err := ctrl.ReadFrame()
		require.NoError(t, err)

		if frame.Type == protocol.MsgHistoryEnd {
			break
		}

		require.Equal(t, protocol.MsgHistory, frame.Type)
		allData = append(allData, frame.Payload...)
	}

	// Text format should have no ANSI escapes.
	assert.NotContains(t, string(allData), "\x1b[")
}

func TestDaemon_History_InvalidFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgHistory, HistoryRequest{
		SessionID: "s1", Format: "xml", Lines: 0,
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_History_SessionNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Invalid format is caught before session lookup, so use a valid format
	// to verify the session-not-found path.
	histReq, _ := json.Marshal(HistoryRequest{
		SessionID: "nonexistent", Format: "ansi", Lines: 0,
	})
	err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgHistory,
		Payload: histReq,
	})
	require.NoError(t, err)

	frame, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, frame.Type)
}

func TestDaemon_ListSessions_PrefixFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create two sessions with prefix and one without.
	for _, id := range []string{"proj-a/s1", "proj-a/s2", "other/s3"} {
		resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
			ID:    id,
			Shell: "/bin/sh",
			Args:  nil,
			Cols:  80,
			Rows:  24,
			Cwd:   "",
			Env:   nil,
		})
		require.Equal(t, protocol.MsgOK, resp.Type, "create %s", id)
	}

	// List with prefix filter.
	listResp := sendControl(t, ctrl, protocol.MsgList, ListRequest{Prefix: "proj-a"})
	require.Equal(t, protocol.MsgOK, listResp.Type)

	var sessions []SessionResponse
	require.NoError(t, json.Unmarshal(listResp.Payload, &sessions))
	assert.Len(t, sessions, 2)
	for _, s := range sessions {
		assert.True(t, strings.HasPrefix(s.ID, "proj-a/"))
	}
}

func TestDaemon_ListSessions_NoFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create sessions.
	for _, id := range []string{"a/s1", "b/s2"} {
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

	// List without filter (nil payload).
	listResp := sendControl(t, ctrl, protocol.MsgList, nil)
	require.Equal(t, protocol.MsgOK, listResp.Type)

	var sessions []SessionResponse
	require.NoError(t, json.Unmarshal(listResp.Payload, &sessions))
	assert.Len(t, sessions, 2)
}

func TestDaemon_KillPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create sessions.
	for _, id := range []string{"proj-a/s1", "proj-a/s2", "proj-b/s3"} {
		resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
			ID: id, Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
		})
		require.Equal(t, protocol.MsgOK, resp.Type, "create %s", id)
	}

	// Kill by prefix.
	killResp := sendControl(t, ctrl, protocol.MsgKillPrefix, KillPrefixRequest{Prefix: "proj-a"})
	require.Equal(t, protocol.MsgOK, killResp.Type)

	var result KillPrefixResponse
	require.NoError(t, json.Unmarshal(killResp.Payload, &result))
	assert.Len(t, result.Killed, 2)
	assert.Empty(t, result.Errors)

	// Verify only proj-b/s3 remains.
	listResp := sendControl(t, ctrl, protocol.MsgList, nil)
	require.Equal(t, protocol.MsgOK, listResp.Type)

	var allSessions []SessionResponse
	require.NoError(t, json.Unmarshal(listResp.Payload, &allSessions))
	// Should have 1 alive + 2 exited = 3, but only 1 with alive state.
	aliveCount := 0
	for _, s := range allSessions {
		if s.State == "alive" || s.State == "detached" {
			aliveCount++
		}
	}
	assert.Equal(t, 1, aliveCount)
}

// --- Shared test doubles for scanDA / scanOSC unit tests ---

// noopSessionManager implements SessionManager with no-ops, suitable for
// unit tests that only exercise scanOSC (which only calls MetaSet).
type noopSessionManager struct{}

func (n *noopSessionManager) Create(_ string, _ SessionCreateOptions) (SessionInfo, error) {
	return SessionInfo{ID: "", State: "", Pid: 0, Cols: 0, Rows: 0, Shell: ""}, nil
}
func (n *noopSessionManager) Get(_ string) (SessionInfo, error) {
	return SessionInfo{ID: "", State: "", Pid: 0, Cols: 0, Rows: 0, Shell: ""}, nil
}
func (n *noopSessionManager) List() []SessionInfo                 { return nil }
func (n *noopSessionManager) Kill(_ string) error                 { return nil }
func (n *noopSessionManager) Resize(_ string, _, _ int) error     { return nil }
func (n *noopSessionManager) WriteInput(_ string, _ []byte) error { return nil }
func (n *noopSessionManager) ReadOutput(_ string) ([]byte, error) { return nil, nil }
func (n *noopSessionManager) Attach(_ string) error               { return nil }
func (n *noopSessionManager) Detach(_ string) error               { return nil }
func (n *noopSessionManager) Snapshot(_ string) (SnapshotData, error) {
	return SnapshotData{Replay: nil}, nil
}
func (n *noopSessionManager) LastActivity(_ string) (time.Time, error)       { return time.Time{}, nil }
func (n *noopSessionManager) MetaSet(_, _, _ string) error                   { return nil }
func (n *noopSessionManager) MetaGet(_, _ string) (string, error)            { return "", nil }
func (n *noopSessionManager) MetaGetAll(_ string) (map[string]string, error) { return nil, nil }
func (n *noopSessionManager) UpdateEmulatorScrollback(_ string, _ int) error { return nil }
func (n *noopSessionManager) OnExit(_ func(id string, exitCode int))         {}

// snapshotSpySessionManager extends noopSessionManager to return configurable
// snapshot data. Used to test handleAttach snapshot population.
type snapshotSpySessionManager struct {
	noopSessionManager
	snapshotData SnapshotData
}

func (s *snapshotSpySessionManager) Get(_ string) (SessionInfo, error) {
	return SessionInfo{
		ID:    "snap-test",
		State: "alive",
		Pid:   12345,
		Cols:  80,
		Rows:  24,
		Shell: "/bin/sh",
	}, nil
}

func (s *snapshotSpySessionManager) Snapshot(_ string) (SnapshotData, error) {
	return s.snapshotData, nil
}

// mockControlConn is a mock ControlConn that records written frames.
type mockControlConn struct {
	frames   []protocol.Frame // frames to return from ReadFrame
	writeMu  sync.Mutex
	written  []protocol.Frame
	writeErr error
}

func (m *mockControlConn) ReadFrame() (protocol.Frame, error) {
	if len(m.frames) == 0 {
		return protocol.Frame{}, errors.New("no frames")
	}
	f := m.frames[0]
	m.frames = m.frames[1:]
	return f, nil
}

func (m *mockControlConn) WriteFrame(f protocol.Frame) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, f)
	return nil
}

// mockConnectedClient is a mock ConnectedClient for unit tests.
type mockConnectedClient struct {
	id   string
	ctrl ControlConn
}

func (m *mockConnectedClient) ClientID() string     { return m.id }
func (m *mockConnectedClient) Control() ControlConn { return m.ctrl }

// spyEventBus records published events for inspection in scanOSC unit tests.
type spyEventBus struct {
	mu     sync.Mutex
	events []event.Event
}

func (b *spyEventBus) Publish(e event.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *spyEventBus) Subscribe() *event.Subscription {
	return nil
}

// writeInputCall records a single WriteInput invocation.
type writeInputCall struct {
	id   string
	data []byte
}

// spySessionManager is a minimal SessionManager spy for scanDA tests.
type spySessionManager struct {
	noopSessionManager // embed noop for unimplemented methods
	mu                 sync.Mutex
	writeInputCalls    []writeInputCall
}

func (s *spySessionManager) WriteInput(id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeInputCalls = append(s.writeInputCalls, writeInputCall{id: id, data: data})
	return nil
}

// newTestDaemonUnit builds a minimal Daemon with all fields populated to
// satisfy the exhaustruct linter. Used for unit-testing individual methods.
func newTestDaemonUnit(sm SessionManager, bus EventBus, attachments map[string]map[string]struct{}) *Daemon {
	return &Daemon{
		mu:                 sync.RWMutex{},
		server:             nil,
		sessionSvc:         sm,
		version:            "",
		pidFilePath:        "",
		dataDir:            "",
		cancelFunc:         nil,
		attachments:        attachments,
		clientSession:      map[string]string{},
		eventBus:           bus,
		startedAt:          time.Time{},
		coldRestore:        false,
		maxScrollbackSize:  0,
		scrollbackWriters:  map[string]*history.Writer{},
		waiters:            map[string][]*waiter{},
		recordings:         map[string]*recording.Writer{},
		recordingMaxSize:   0,
		recordingDir:       "",
		maxHistoryDumpSize: 0,
		tracer:             nil,
	}
}

// --- scanDA unit tests ---

func TestDaemon_scanDA_InjectsWhenDetached(t *testing.T) {
	spy := &spySessionManager{noopSessionManager: noopSessionManager{}, mu: sync.Mutex{}, writeInputCalls: nil}
	d := newTestDaemonUnit(spy, nil, map[string]map[string]struct{}{})

	d.scanDA("s1", []byte("output\x1b[cmore"))

	spy.mu.Lock()
	calls := spy.writeInputCalls
	spy.mu.Unlock()

	assert.Len(t, calls, 1)
	assert.Equal(t, "s1", calls[0].id)
	assert.Equal(t, DA1Response(), calls[0].data)
}

func TestDaemon_scanDA_SkipsWhenAttached(t *testing.T) {
	spy := &spySessionManager{noopSessionManager: noopSessionManager{}, mu: sync.Mutex{}, writeInputCalls: nil}
	d := newTestDaemonUnit(spy, nil, map[string]map[string]struct{}{
		"s1": {"client-1": {}},
	})

	d.scanDA("s1", []byte("\x1b[c"))

	spy.mu.Lock()
	calls := spy.writeInputCalls
	spy.mu.Unlock()

	assert.Empty(t, calls)
}

func TestDaemon_scanDA_DA2(t *testing.T) {
	spy := &spySessionManager{noopSessionManager: noopSessionManager{}, mu: sync.Mutex{}, writeInputCalls: nil}
	d := newTestDaemonUnit(spy, nil, map[string]map[string]struct{}{})

	d.scanDA("s1", []byte("\x1b[>c"))

	spy.mu.Lock()
	calls := spy.writeInputCalls
	spy.mu.Unlock()

	assert.Len(t, calls, 1)
	assert.Equal(t, DA2Response(), calls[0].data)
}

func TestDaemon_scanDA_BothDA1andDA2(t *testing.T) {
	spy := &spySessionManager{noopSessionManager: noopSessionManager{}, mu: sync.Mutex{}, writeInputCalls: nil}
	d := newTestDaemonUnit(spy, nil, map[string]map[string]struct{}{})

	d.scanDA("s1", []byte("\x1b[c\x1b[>c"))

	spy.mu.Lock()
	calls := spy.writeInputCalls
	spy.mu.Unlock()

	assert.Len(t, calls, 2)
	assert.Equal(t, DA1Response(), calls[0].data)
	assert.Equal(t, DA2Response(), calls[1].data)
}

func TestDaemon_scanDA_NoSequences(t *testing.T) {
	spy := &spySessionManager{noopSessionManager: noopSessionManager{}, mu: sync.Mutex{}, writeInputCalls: nil}
	d := newTestDaemonUnit(spy, nil, map[string]map[string]struct{}{})

	d.scanDA("s1", []byte("plain text output"))

	spy.mu.Lock()
	calls := spy.writeInputCalls
	spy.mu.Unlock()

	assert.Empty(t, calls)
}

// --- scanOSC unit tests ---

func TestDaemon_scanOSC_ShellReady(t *testing.T) {
	bus := &spyEventBus{mu: sync.Mutex{}, events: nil}
	d := newTestDaemonUnit(&noopSessionManager{}, bus, map[string]map[string]struct{}{})

	data := []byte("\x1b]777;wmux;shell-ready\x1b\\")
	d.scanOSC("s1", data)

	require.Len(t, bus.events, 1)
	assert.Equal(t, event.ShellReady, bus.events[0].Type)
	assert.Equal(t, "s1", bus.events[0].SessionID)
	assert.Equal(t, map[string]any{}, bus.events[0].Payload)
}

func TestDaemon_KillPrefix_NoMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	// Create a session with different prefix.
	resp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID: "proj-b/s1", Shell: "/bin/sh", Args: nil, Cols: 80, Rows: 24, Cwd: "", Env: nil,
	})
	require.Equal(t, protocol.MsgOK, resp.Type)

	// Kill non-existent prefix → error.
	killResp := sendControl(t, ctrl, protocol.MsgKillPrefix, KillPrefixRequest{Prefix: "nonexistent"})
	assert.Equal(t, protocol.MsgError, killResp.Type)
}

func TestDaemon_UpdateEmulatorScrollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	createResp := sendControl(t, ctrl, protocol.MsgCreate, CreateRequest{
		ID:    "scroll-sess",
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.Equal(t, protocol.MsgOK, createResp.Type)

	// NoneEmulator doesn't implement ScrollbackConfigurable, so this should error.
	resp := sendControl(t, ctrl, protocol.MsgUpdateEmulatorScrollback, UpdateEmulatorScrollbackRequest{
		SessionID:       "scroll-sess",
		ScrollbackLines: 50000,
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
	assert.Contains(t, string(resp.Payload), "scrollback")
}

func TestDaemon_UpdateEmulatorScrollback_NotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, token, sock := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	ctrl, _ := dialControl(t, sock, token)
	defer ctrl.Close() //nolint:errcheck

	resp := sendControl(t, ctrl, protocol.MsgUpdateEmulatorScrollback, UpdateEmulatorScrollbackRequest{
		SessionID:       "no-such",
		ScrollbackLines: 10000,
	})
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestDaemon_UpdateEmulatorScrollback_InvalidPayload(t *testing.T) {
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
		Type:    protocol.MsgUpdateEmulatorScrollback,
		Payload: []byte("{bad json"),
	})
	require.NoError(t, err)

	resp, err := ctrl.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, protocol.MsgError, resp.Type)
}
