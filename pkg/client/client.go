package client

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/transport"
)

// ErrRequestTimeout is returned when an RPC does not receive a response
// within the configured timeout.
var ErrRequestTimeout = errors.New("client: request timeout")

// Client is a connection to a wmux daemon.
type Client struct {
	mu    sync.Mutex // protects handler fields
	rpcMu sync.Mutex // serializes RPC write+wait pairs

	clientID   string
	rpcTimeout time.Duration

	ctrlConn   net.Conn
	ctrl       *protocol.Conn
	streamConn net.Conn
	stream     *protocol.Conn

	done      chan struct{}
	responses chan protocol.Frame
	history   chan protocol.Frame

	// drainWg tracks in-flight drain goroutines launched after a timeout.
	// The next sendRequest waits for all drains to finish before writing,
	// preventing a drain goroutine from consuming a response that belongs
	// to the subsequent RPC.
	drainWg sync.WaitGroup

	dataHandler func(sessionID string, data []byte)
	evtHandler  func(Event)
}

// New establishes a connection to the wmux daemon, authenticates, and
// returns a ready-to-use Client. If no daemon is running and autoStart is
// true (the default), it spawns one automatically and waits for it to be ready.
func New(opts ...Option) (*Client, error) {
	cfg := newConfig(opts...)
	resolveConfig(cfg)

	c, err := connect(cfg)
	if err == nil {
		return c, nil
	}

	if !cfg.autoStart {
		return nil, err
	}

	if err := startDaemon(cfg); err != nil {
		return nil, fmt.Errorf("client: auto-start: %w", err)
	}

	return connect(cfg)
}

// startDaemon spawns a daemon process using the current executable.
func startDaemon(cfg *config) error {
	nsDir := filepath.Dir(cfg.socket)
	if err := os.MkdirAll(nsDir, 0o700); err != nil {
		return fmt.Errorf("create dirs: %w", err)
	}
	if err := os.MkdirAll(cfg.dataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	if _, err := auth.Ensure(cfg.tokenPath); err != nil {
		return fmt.Errorf("ensure token: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	args := buildDaemonArgs(cfg)
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} //nolint:exhaustruct
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	// Detach — don't wait for the child
	_ = cmd.Process.Release()

	// Poll until socket is ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", cfg.socket, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("daemon did not start within 3s")
}

// buildDaemonArgs creates the CLI args for spawning a daemon subprocess.
func buildDaemonArgs(cfg *config) []string {
	args := []string{daemonSentinel}

	if cfg.baseDir != "" {
		args = append(args, "--base-dir", cfg.baseDir)
	}
	if cfg.namespace != "default" {
		args = append(args, "--namespace", cfg.namespace)
	}
	args = append(args, "--socket", cfg.socket)
	args = append(args, "--token-path", cfg.tokenPath)
	args = append(args, "--data-dir", cfg.dataDir)

	if cfg.coldRestore {
		args = append(args, "--cold-restore")
	}
	if cfg.maxScrollbackSize > 0 {
		args = append(args, "--max-scrollback", strconv.FormatInt(cfg.maxScrollbackSize, 10))
	}
	return args
}

// connect dials the daemon and performs the auth handshake using the given config.
func connect(cfg *config) (*Client, error) {
	token, err := auth.LoadFromFile(cfg.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("client: read token: %w", err)
	}

	// Step 1: open control channel.
	ctrlConn, err := net.Dial("unix", cfg.socket)
	if err != nil {
		return nil, fmt.Errorf("client: dial control: %w", err)
	}

	ctrl := protocol.NewConn(ctrlConn)

	payload := make([]byte, 0, 1+auth.TokenSize)
	payload = append(payload, byte(transport.ChannelControl))
	payload = append(payload, token...)

	if err := ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	}); err != nil {
		_ = ctrlConn.Close()
		return nil, fmt.Errorf("client: auth write: %w", err)
	}

	frame, err := ctrl.ReadFrame()
	if err != nil {
		_ = ctrlConn.Close()
		return nil, fmt.Errorf("client: auth read: %w", err)
	}

	if frame.Type != protocol.MsgOK {
		_ = ctrlConn.Close()
		return nil, fmt.Errorf("client: auth failed")
	}

	clientID := string(frame.Payload)

	// Step 2: open stream channel.
	streamConn, err := dialStream(cfg.socket, token, clientID)
	if err != nil {
		_ = ctrlConn.Close()
		return nil, fmt.Errorf("client: stream: %w", err)
	}

	c := &Client{
		mu:          sync.Mutex{},
		rpcMu:       sync.Mutex{},
		drainWg:     sync.WaitGroup{},
		clientID:    clientID,
		rpcTimeout:  cfg.rpcTimeout,
		ctrlConn:    ctrlConn,
		ctrl:        ctrl,
		streamConn:  streamConn,
		stream:      protocol.NewConn(streamConn),
		done:        make(chan struct{}),
		responses:   make(chan protocol.Frame, 1),
		history:     make(chan protocol.Frame, 4),
		dataHandler: nil,
		evtHandler:  nil,
	}

	// Step 3: spawn reader goroutines.
	go c.readStream()
	go c.readControl()

	// Step 4: subscribe to daemon events (standard RPC via readers).
	if err := c.subscribe(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("client: subscribe: %w", err)
	}

	return c, nil
}

// Close closes both connections to the daemon.
func (c *Client) Close() error {
	close(c.done)

	ctrlErr := c.ctrlConn.Close()
	streamErr := c.streamConn.Close()

	if ctrlErr != nil {
		return fmt.Errorf("client: close control: %w", ctrlErr)
	}
	if streamErr != nil {
		return fmt.Errorf("client: close stream: %w", streamErr)
	}
	return nil
}

// sendRequest sends a control frame and waits for the demuxed response.
// Returns ErrRequestTimeout if the daemon does not respond within rpcTimeout.
func (c *Client) sendRequest(msgType protocol.MessageType, payload []byte) (protocol.Frame, error) {
	// Wait for any in-flight drain goroutine from a previous timeout to finish
	// before acquiring rpcMu. This prevents the drain from consuming the
	// response that belongs to this RPC. The wait is outside the lock so that
	// drain goroutines can run while we block here.
	c.drainWg.Wait()

	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()

	if err := c.ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Payload: payload,
	}); err != nil {
		return protocol.Frame{}, fmt.Errorf("client: write: %w", err)
	}

	select {
	case resp, ok := <-c.responses:
		if !ok {
			return protocol.Frame{}, fmt.Errorf("client: connection closed")
		}
		return resp, nil

	case <-time.After(c.rpcTimeout):
		// Drain the stale response in the background so the next RPC
		// does not receive a mismatched response from this timed-out request.
		c.drainWg.Add(1)
		go func() {
			defer c.drainWg.Done()
			select {
			case <-c.responses:
			case <-c.done:
			}
		}()
		return protocol.Frame{}, ErrRequestTimeout
	}
}
