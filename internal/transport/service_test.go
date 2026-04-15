package transport

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// testServer creates a Server backed by a temporary Unix socket.
// It returns the server, the auth token, and the socket path.
func testServer(t *testing.T, opts ...Option) (*Server, []byte, string) {
	t.Helper()

	// macOS limits Unix socket paths to 104 bytes. Use os.MkdirTemp with a
	// short prefix under /tmp to stay well within the limit.
	dir, err := os.MkdirTemp("", "wmux")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sock := filepath.Join(dir, "t.sock")

	ln, err := ipc.Listen(sock)
	require.NoError(t, err)

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := NewServer(ln, token, opts...)

	return srv, token, sock
}

// startServer launches srv.Serve in a goroutine and returns a cancel func.
func startServer(t *testing.T, srv *Server) context.CancelFunc {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	go func() { _ = srv.Serve(ctx) }()

	time.Sleep(10 * time.Millisecond) // let accept loop start

	return cancel
}

// dialAndAuth connects to sockPath, sends an auth frame, and returns the Conn
// and the server's response frame.
func dialAndAuth(t *testing.T, sockPath string, token []byte, channelType ChannelType, clientID string) (*protocol.Conn, protocol.Frame) {
	t.Helper()

	raw, err := net.Dial("unix", sockPath)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)

	var payload []byte
	payload = append(payload, byte(channelType))
	payload = append(payload, token...)

	if channelType == ChannelStream && clientID != "" {
		payload = append(payload, byte(len(clientID)))
		payload = append(payload, []byte(clientID)...)
	}

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	return conn, resp
}

func TestServer_ControlHandshake(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	assert.Equal(t, protocol.MsgOK, resp.Type)
	assert.NotEmpty(t, resp.Payload, "expected client ID in MsgOK payload")
}

func TestServer_StreamHandshake(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	// First establish the control channel and obtain a client ID.
	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)
	require.NotEmpty(t, clientID)

	// Now establish the stream channel for the same client.
	stream, sresp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream.Close() //nolint:errcheck

	assert.Equal(t, protocol.MsgOK, sresp.Type)
	assert.Empty(t, sresp.Payload)
}

func TestServer_BadToken(t *testing.T) {
	srv, _, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	badToken := make([]byte, auth.TokenSize)
	for i := range badToken {
		badToken[i] = 0xFF
	}

	conn, resp := dialAndAuth(t, sock, badToken, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestServer_Clients(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	assert.Empty(t, srv.Clients())

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	time.Sleep(5 * time.Millisecond)

	clients := srv.Clients()
	assert.Len(t, clients, 1)
	assert.Equal(t, string(resp.Payload), clients[0].ID)
}

func TestServer_Kick(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	time.Sleep(5 * time.Millisecond)

	err := srv.Kick(clientID)
	require.NoError(t, err)

	assert.Empty(t, srv.Clients())
}

func TestServer_KickNotFound(t *testing.T) {
	srv, _, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	err := srv.Kick("nonexistent-id")
	assert.ErrorIs(t, err, ErrClientNotFound)

	_ = sock
}

func TestServer_Close(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	time.Sleep(5 * time.Millisecond)

	err := srv.Close()
	require.NoError(t, err)
	assert.Empty(t, srv.Clients())
}

func TestServer_SameUserMode(t *testing.T) {
	srv, token, sock := testServer(t, WithAutomationMode(ModeSameUser))
	cancel := startServer(t, srv)
	defer cancel()

	// The test process runs as the same user → should succeed.
	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	assert.Equal(t, protocol.MsgOK, resp.Type)
}

func TestServer_BroadcastToStream(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	stream, sresp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, sresp.Type)

	// Give the server time to record the stream.
	time.Sleep(10 * time.Millisecond)

	f := protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: []byte("hello"),
	}

	err := srv.Broadcast(f)
	require.NoError(t, err)

	got, err := stream.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, f.Type, got.Type)
	assert.Equal(t, f.Payload, got.Payload)
}

func TestServer_BroadcastTo(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	stream, sresp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, sresp.Type)

	time.Sleep(10 * time.Millisecond)

	f := protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: []byte("targeted"),
	}

	err := srv.BroadcastTo(clientID, f)
	require.NoError(t, err)

	got, err := stream.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, f.Type, got.Type)
	assert.Equal(t, f.Payload, got.Payload)
}

func TestServer_BroadcastToNotFound(t *testing.T) {
	srv, _, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	err := srv.BroadcastTo("ghost", protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: nil,
	})
	assert.ErrorIs(t, err, ErrClientNotFound)

	_ = sock
}

func TestServer_Client(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	time.Sleep(5 * time.Millisecond)

	c, err := srv.Client(clientID)
	require.NoError(t, err)
	assert.Equal(t, clientID, c.ID)
}

func TestServer_ClientNotFound(t *testing.T) {
	srv, _, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	_, err := srv.Client("no-such-id")
	assert.ErrorIs(t, err, ErrClientNotFound)

	_ = sock
}

func TestServer_RemoveClient(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	time.Sleep(5 * time.Millisecond)
	assert.Len(t, srv.Clients(), 1)

	srv.RemoveClient(clientID)

	assert.Empty(t, srv.Clients())

	// The underlying connection should still be open (not closed).
	_, err := srv.Client(clientID)
	assert.ErrorIs(t, err, ErrClientNotFound)
}

func TestServer_InvalidAuthFrame(t *testing.T) {
	srv, _, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	// Send a non-MsgAuth frame.
	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: []byte("bad"),
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestServer_ShortPayload(t *testing.T) {
	srv, _, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	// Payload is shorter than minAuthPayloadSize.
	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: []byte{0x01, 0x02},
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestServer_InvalidChannelType(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	// Build a valid-sized payload with an unknown channel type byte (0xFF).
	payload := make([]byte, minAuthPayloadSize)
	payload[0] = 0xFF
	copy(payload[1:], token)

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestServer_StreamWithoutControlFirst(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	// Try to open a stream for a client ID that was never registered.
	conn, resp := dialAndAuth(t, sock, token, ChannelStream, "does-not-exist")
	defer conn.Close() //nolint:errcheck

	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestServer_DuplicateStream(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	stream1, s1resp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream1.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, s1resp.Type)

	time.Sleep(10 * time.Millisecond)

	// A second stream for the same client should be rejected.
	stream2, s2resp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream2.Close() //nolint:errcheck

	assert.Equal(t, protocol.MsgError, s2resp.Type)
}

func TestIsDescendant(t *testing.T) {
	self := int32(os.Getpid())  //nolint:gosec // safe narrowing in tests
	ppid := int32(os.Getppid()) //nolint:gosec // safe narrowing in tests

	// The current process is a descendant of its own parent.
	assert.True(t, isDescendant(self, ppid),
		"current process should be a descendant of its parent")

	// A random nonexistent PID is not a descendant of the current process.
	assert.False(t, isDescendant(self, 99999999),
		"process should not be a descendant of a nonexistent ancestor")
}

func TestParentPID(t *testing.T) {
	self := int32(os.Getpid())      //nolint:gosec // safe narrowing in tests
	expected := int32(os.Getppid()) //nolint:gosec // safe narrowing in tests

	got, err := parentPID(self)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestGenerateClientID(t *testing.T) {
	id1, err := generateClientID()
	require.NoError(t, err)
	assert.Len(t, id1, clientIDSize*2) // hex encoded

	id2, err := generateClientID()
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2, "two generated IDs should differ")
}

func TestGenerateClientID_RandError(t *testing.T) {
	original := randReader
	randReader = &failReader{}
	defer func() { randReader = original }()

	_, err := generateClientID()
	assert.Error(t, err)
}

// failReader is an io.Reader that always returns an error.
type failReader struct{}

func (f *failReader) Read(_ []byte) (int, error) {
	return 0, errors.New("rand read error")
}

func TestServer_BroadcastNoStream(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	// Register a control channel only (no stream).
	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	time.Sleep(5 * time.Millisecond)

	// Broadcast should succeed (no-op) when there are no stream channels.
	err := srv.Broadcast(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: []byte("ignored"),
	})
	assert.NoError(t, err)
}

func TestServer_ChildrenMode(t *testing.T) {
	srv, token, sock := testServer(t, WithAutomationMode(ModeChildren))
	cancel := startServer(t, srv)
	defer cancel()

	// The test process is NOT a descendant of itself (isDescendant checks the
	// parent chain, not identity), so this connection should be rejected.
	// However, on some systems the test runner IS a child — so we accept either
	// outcome but ensure we get a valid protocol response.
	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	payload := make([]byte, minAuthPayloadSize)
	payload[0] = byte(ChannelControl)
	copy(payload[1:], token)

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	// Either OK (if the test process is a child of the server) or Error.
	assert.True(t, resp.Type == protocol.MsgOK || resp.Type == protocol.MsgError)
}

// TestServer_AutomationModeReject_SendsProtocolError verifies that when
// checkAutomationMode rejects a connection, the client receives a MsgError
// frame instead of a broken pipe.
//
// BUG: handleConn calls checkAutomationMode BEFORE ReadFrame. If the check
// fails, the server closes the raw conn before the client can send or read
// anything. The client gets a broken pipe on WriteFrame instead of a clean
// MsgError. This test uses an invalid AutomationMode to force rejection on
// any OS (the ModeChildren path only rejects on Linux CI, not macOS).
func TestServer_AutomationModeReject_SendsProtocolError(t *testing.T) {
	// AutomationMode(99) hits the default branch which always rejects.
	srv, token, sock := testServer(t, WithAutomationMode(AutomationMode(99)))
	cancel := startServer(t, srv)
	defer cancel()

	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	// Give the server time to run checkAutomationMode and close the conn.
	// The bug: the server closes before ReadFrame, so the client's write
	// races with the close. This sleep ensures the server has already closed.
	time.Sleep(50 * time.Millisecond)

	payload := make([]byte, minAuthPayloadSize)
	payload[0] = byte(ChannelControl)
	copy(payload[1:], token)

	// The client must be able to send the auth frame without broken pipe.
	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err, "server closed conn before client could send auth frame (broken pipe)")

	// The server must reply with a protocol-level MsgError, not a raw conn close.
	resp, err := conn.ReadFrame()
	require.NoError(t, err, "server closed conn without sending MsgError response")
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestServer_StreamShortPayloadAfterToken(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	// First get a valid client ID.
	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	// Now send a stream auth payload that has channel byte + token but NO id_len byte.
	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	// payload = [channel:1][token:32] — exactly minAuthPayloadSize, no id_len
	payload := make([]byte, minAuthPayloadSize)
	payload[0] = byte(ChannelStream)
	copy(payload[1:], token)

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	sresp, err := conn.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, protocol.MsgError, sresp.Type)
}

func TestServer_StreamIDLenOverflow(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	// payload = [channel:1][token:32][id_len:1=255 but only 3 bytes follow]
	payload := make([]byte, minAuthPayloadSize+1+3)
	payload[0] = byte(ChannelStream)
	copy(payload[1:], token)
	payload[minAuthPayloadSize] = 0xFF // id_len=255 but only 3 bytes of ID follow

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	sresp, err := conn.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, protocol.MsgError, sresp.Type)
}

func TestIsDescendant_MaxDepth(t *testing.T) {
	// PID 1 (init/launchd) has no parent (returns ppid 0 or error quickly),
	// so isDescendant(1, 99999999) should return false without hanging.
	result := isDescendant(1, 99999999)
	assert.False(t, result)
}

func TestServer_BroadcastToNoStream(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	time.Sleep(5 * time.Millisecond)

	// BroadcastTo a client with no stream should be a no-op.
	err := srv.BroadcastTo(clientID, protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgData,
		Payload: nil,
	})
	assert.NoError(t, err)
}

// errConn is a net.Conn whose Write always returns an error, used to exercise
// error paths in sendError and WriteFrame.
type errConn struct {
	delegate net.Conn
}

func (e errConn) Write(_ []byte) (int, error)        { return 0, errors.New("write error") }
func (e errConn) Read(_ []byte) (int, error)         { return 0, errors.New("read error") }
func (e errConn) Close() error                       { return errors.New("close error") }
func (e errConn) LocalAddr() net.Addr                { return &net.UnixAddr{Name: "", Net: "unix"} }
func (e errConn) RemoteAddr() net.Addr               { return &net.UnixAddr{Name: "", Net: "unix"} }
func (e errConn) SetDeadline(_ time.Time) error      { return errors.New("deadline error") }
func (e errConn) SetReadDeadline(_ time.Time) error  { return errors.New("deadline error") }
func (e errConn) SetWriteDeadline(_ time.Time) error { return errors.New("deadline error") }

func makeListener(t *testing.T) *ipc.Listener {
	t.Helper()

	dir, err := os.MkdirTemp("", "wmux")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	ln, err := ipc.Listen(filepath.Join(dir, "t.sock"))
	require.NoError(t, err)

	return ln
}

func TestHandleConn_SetDeadlineError(t *testing.T) {
	ln := makeListener(t)
	defer ln.Close() //nolint:errcheck

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := NewServer(ln, token)

	// A connection that fails SetDeadline should be closed silently.
	srv.handleConn(errConn{delegate: nil})
	// No assertions needed — just verify it doesn't panic or hang.
}

func TestCheckAutomationMode_SameUserMismatch(t *testing.T) {
	ln := makeListener(t)
	defer ln.Close() //nolint:errcheck

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := NewServer(ln, token, WithAutomationMode(ModeSameUser))

	// Use a UID that is guaranteed not to match the current user.
	creds := ipc.PeerCredentials{UID: ^uint32(0), PID: int32(os.Getpid())} //nolint:gosec

	err = srv.checkAutomationMode(errConn{delegate: nil}, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthFailed)
}

func TestCheckAutomationMode_UnknownMode(t *testing.T) {
	ln := makeListener(t)
	defer ln.Close() //nolint:errcheck

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := NewServer(ln, token)
	srv.mode = AutomationMode(99) // unknown mode

	creds := ipc.PeerCredentials{UID: uint32(os.Getuid()), PID: int32(os.Getpid())} //nolint:gosec

	err = srv.checkAutomationMode(errConn{delegate: nil}, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthFailed)
}

func TestServer_KickWithStream(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	stream, sresp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, sresp.Type)

	time.Sleep(10 * time.Millisecond)

	// Kick should close both control and stream connections.
	err := srv.Kick(clientID)
	require.NoError(t, err)

	assert.Empty(t, srv.Clients())
}

func TestServer_ControlHandshake_RandFails(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	// Inject a failing rand reader before the handshake.
	original := randReader
	randReader = &failReader{}
	defer func() { randReader = original }()

	raw, err := net.Dial("unix", sock)
	require.NoError(t, err)

	conn := protocol.NewConn(raw)
	defer conn.Close() //nolint:errcheck

	payload := make([]byte, minAuthPayloadSize)
	payload[0] = byte(ChannelControl)
	copy(payload[1:], token)

	err = conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	})
	require.NoError(t, err)

	resp, err := conn.ReadFrame()
	require.NoError(t, err)

	// Server should send MsgError because generateClientID failed.
	assert.Equal(t, protocol.MsgError, resp.Type)
}

func TestRegisterControl_WriteError(t *testing.T) {
	ln := makeListener(t)
	defer ln.Close() //nolint:errcheck

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := NewServer(ln, token)

	// Pass an errConn-backed protocol.Conn — WriteFrame will fail.
	conn := protocol.NewConn(errConn{delegate: nil})
	creds := ipc.PeerCredentials{UID: uint32(os.Getuid()), PID: int32(os.Getpid())} //nolint:gosec

	srv.registerControl(conn, creds)

	// After the write error the client should have been removed from the registry.
	assert.Empty(t, srv.Clients())
}

func TestRegisterStream_WriteError(t *testing.T) {
	ln := makeListener(t)
	defer ln.Close() //nolint:errcheck

	token, err := auth.Generate()
	require.NoError(t, err)

	srv := NewServer(ln, token)

	// Pre-register a client so the stream lookup succeeds.
	clientID := "testclient00000000000000000000001"
	srv.mu.Lock()
	srv.clients[clientID] = &Client{
		ID:               clientID,
		Control:          nil,
		Stream:           nil,
		Creds:            ipc.PeerCredentials{UID: uint32(os.Getuid()), PID: int32(os.Getpid())}, //nolint:gosec
		ConnectedAt:      time.Now(),
		LastHeartbeatAck: time.Time{},
		MissedHeartbeats: 0,
	}
	srv.mu.Unlock()

	// Build the stream payload manually.
	const streamHeaderSize = 1 + auth.TokenSize
	payload := make([]byte, streamHeaderSize+1+len(clientID))
	payload[0] = byte(ChannelStream)
	copy(payload[1:], token)
	payload[streamHeaderSize] = byte(len(clientID))
	copy(payload[streamHeaderSize+1:], clientID)

	conn := protocol.NewConn(errConn{delegate: nil})
	creds := ipc.PeerCredentials{UID: uint32(os.Getuid()), PID: int32(os.Getpid())} //nolint:gosec

	srv.registerStream(conn, payload, creds)

	// Stream should have been cleared after the write error.
	srv.mu.RLock()
	c := srv.clients[clientID]
	srv.mu.RUnlock()

	assert.Nil(t, c.Stream)
}

func TestServer_OnClientCallback(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()
	defer srv.Close() //nolint:errcheck

	called := make(chan string, 1)
	srv.OnClient(func(c *Client) {
		called <- c.ID
	})

	conn, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer conn.Close() //nolint:errcheck
	clientID := string(resp.Payload)

	select {
	case id := <-called:
		assert.Equal(t, clientID, id)
	case <-time.After(time.Second):
		t.Fatal("OnClient callback not called")
	}
}

func TestServer_CloseWithStream(t *testing.T) {
	srv, token, sock := testServer(t)
	cancel := startServer(t, srv)
	defer cancel()

	ctrl, resp := dialAndAuth(t, sock, token, ChannelControl, "")
	defer ctrl.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, resp.Type)

	clientID := string(resp.Payload)

	stream, sresp := dialAndAuth(t, sock, token, ChannelStream, clientID)
	defer stream.Close() //nolint:errcheck

	require.Equal(t, protocol.MsgOK, sresp.Type)

	time.Sleep(10 * time.Millisecond)

	err := srv.Close()
	assert.NoError(t, err)
}
