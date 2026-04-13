package client

import (
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

// Client is a connection to a wmux daemon.
type Client struct {
	mu          sync.Mutex
	conn        net.Conn
	pConn       *protocol.Conn
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
	if cfg.emulatorBackend != "none" {
		args = append(args, "--emulator-backend", cfg.emulatorBackend)
	}

	return args
}

// connect dials the daemon and performs the auth handshake using the given config.
func connect(cfg *config) (*Client, error) {
	token, err := auth.LoadFromFile(cfg.tokenPath)
	if err != nil {
		return nil, fmt.Errorf("client: read token: %w", err)
	}

	conn, err := net.Dial("unix", cfg.socket)
	if err != nil {
		return nil, fmt.Errorf("client: dial: %w", err)
	}

	pConn := protocol.NewConn(conn)

	payload := make([]byte, 0, 1+auth.TokenSize)
	payload = append(payload, byte(transport.ChannelControl))
	payload = append(payload, token...)

	if err := pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: auth write: %w", err)
	}

	frame, err := pConn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("client: auth read: %w", err)
	}

	if frame.Type != protocol.MsgOK {
		_ = conn.Close()
		return nil, fmt.Errorf("client: auth failed")
	}

	return &Client{
		mu:          sync.Mutex{},
		conn:        conn,
		pConn:       pConn,
		dataHandler: nil,
		evtHandler:  nil,
	}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("client: close: %w", err)
	}
	return nil
}

// sendRequest sends a control frame and reads the response.
func (c *Client) sendRequest(msgType protocol.MessageType, payload []byte) (protocol.Frame, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Payload: payload,
	}); err != nil {
		return protocol.Frame{}, fmt.Errorf("client: write: %w", err)
	}

	resp, err := c.pConn.ReadFrame()
	if err != nil {
		return protocol.Frame{}, fmt.Errorf("client: read: %w", err)
	}

	return resp, nil
}
