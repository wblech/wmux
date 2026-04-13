package client

import (
	"fmt"
	"net"
	"sync"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
)

// Client is a connection to a wmux daemon.
type Client struct {
	mu          sync.Mutex
	conn        net.Conn
	pConn       *protocol.Conn
	dataHandler func(sessionID string, data []byte)
	evtHandler  func(Event)
}

// Connect establishes a connection to the wmux daemon, authenticates, and
// returns a ready-to-use Client.
func Connect(opts Options) (*Client, error) {
	token, err := auth.LoadFromFile(opts.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("client: read token: %w", err)
	}

	conn, err := net.Dial("unix", opts.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("client: dial: %w", err)
	}

	pConn := protocol.NewConn(conn)

	if err := pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: token,
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: auth write: %w", err)
	}

	frame, err := pConn.ReadFrame()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: auth read: %w", err)
	}

	if frame.Type != protocol.MsgOK {
		conn.Close()
		return nil, fmt.Errorf("client: auth failed")
	}

	return &Client{
		conn:  conn,
		pConn: pConn,
	}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	return c.conn.Close()
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
