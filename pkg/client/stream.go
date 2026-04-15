package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/transport"
)

// errDataPayloadTooShort is returned when a MsgData payload is malformed.
var errDataPayloadTooShort = errors.New("client: data payload too short")

// decodeDataPayload decodes a binary MsgData payload: [id_len:1][id:N][data:rest].
func decodeDataPayload(payload []byte) (sessionID string, data []byte, err error) {
	if len(payload) < 1 {
		return "", nil, errDataPayloadTooShort
	}
	idLen := int(payload[0])
	if len(payload) < 1+idLen {
		return "", nil, errDataPayloadTooShort
	}
	return string(payload[1 : 1+idLen]), payload[1+idLen:], nil
}

// dialStream opens a stream channel connection to the daemon.
func dialStream(socket string, token []byte, clientID string) (net.Conn, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("dial stream: %w", err)
	}

	pConn := protocol.NewConn(conn)

	payload := make([]byte, 0, 1+auth.TokenSize+1+len(clientID))
	payload = append(payload, byte(transport.ChannelStream))
	payload = append(payload, token...)
	payload = append(payload, byte(len(clientID)))
	payload = append(payload, []byte(clientID)...)

	if err := pConn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("auth write: %w", err)
	}

	frame, err := pConn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("auth read: %w", err)
	}

	if frame.Type != protocol.MsgOK {
		_ = conn.Close()
		return nil, fmt.Errorf("auth rejected")
	}

	return conn, nil
}

// readStream reads MsgData frames from the stream channel and dispatches to dataHandler.
func (c *Client) readStream() {
	for {
		frame, err := c.stream.ReadFrame()
		if err != nil {
			return
		}

		if frame.Type != protocol.MsgData {
			continue
		}

		sessionID, data, err := decodeDataPayload(frame.Payload)
		if err != nil {
			continue
		}

		c.mu.Lock()
		h := c.dataHandler
		c.mu.Unlock()

		if h != nil {
			h(sessionID, data)
		}
	}
}

// readControl demuxes the control channel: events → evtHandler,
// history frames → history channel, everything else → responses channel.
func (c *Client) readControl() {
	defer close(c.responses)
	defer close(c.history)

	for {
		frame, err := c.ctrl.ReadFrame()
		if err != nil {
			return
		}

		switch frame.Type {
		case protocol.MsgEvent:
			c.mu.Lock()
			h := c.evtHandler
			c.mu.Unlock()
			if h != nil {
				h(decodeEvent(frame.Payload))
			}
		case protocol.MsgHistory, protocol.MsgHistoryEnd:
			c.history <- frame
		default:
			c.responses <- frame
		}
	}
}

// decodeEvent unmarshals a JSON event payload.
func decodeEvent(payload []byte) Event {
	var evt Event
	_ = json.Unmarshal(payload, &evt)
	return evt
}
