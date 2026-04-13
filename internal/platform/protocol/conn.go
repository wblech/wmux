package protocol

import (
	"fmt"
	"net"
	"sync"
)

// Conn wraps a net.Conn with frame-based read/write using the binary protocol.
// ReadFrame and WriteFrame are each individually safe for concurrent use,
// but concurrent reads or concurrent writes require external synchronization.
type Conn struct {
	raw   net.Conn
	codec Codec
	rmu   sync.Mutex
	wmu   sync.Mutex
}

// NewConn creates a Conn that reads and writes frames over c.
func NewConn(c net.Conn) *Conn {
	return &Conn{
		raw:   c,
		codec: Codec{},
		rmu:   sync.Mutex{},
		wmu:   sync.Mutex{},
	}
}

// ReadFrame reads and decodes one frame from the connection.
func (c *Conn) ReadFrame() (Frame, error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()

	f, err := c.codec.Decode(c.raw)
	if err != nil {
		return Frame{}, fmt.Errorf("conn read: %w", err)
	}

	return f, nil
}

// WriteFrame encodes and writes one frame to the connection.
func (c *Conn) WriteFrame(f Frame) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	if err := c.codec.Encode(c.raw, f); err != nil {
		return fmt.Errorf("conn write: %w", err)
	}

	return nil
}

// Raw returns the underlying net.Conn.
func (c *Conn) Raw() net.Conn {
	return c.raw
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	if err := c.raw.Close(); err != nil {
		return fmt.Errorf("conn close: %w", err)
	}

	return nil
}
