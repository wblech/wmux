package client

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// RecordResult holds the response from a record start/stop operation.
type RecordResult struct {
	SessionID string `json:"session_id"`
	Recording bool   `json:"recording"`
	Path      string `json:"path,omitempty"`
}

// RecordStart begins recording a session. Returns the recording status and path.
func (c *Client) RecordStart(sessionID string) (RecordResult, error) {
	return c.recordAction(sessionID, "start")
}

// RecordStop stops recording a session.
func (c *Client) RecordStop(sessionID string) (RecordResult, error) {
	return c.recordAction(sessionID, "stop")
}

func (c *Client) recordAction(sessionID, action string) (RecordResult, error) {
	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
	}{
		SessionID: sessionID,
		Action:    action,
	})
	if err != nil {
		return RecordResult{}, fmt.Errorf("client: marshal record: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgRecord, payload)
	if err != nil {
		return RecordResult{}, err
	}
	if resp.Type == protocol.MsgError {
		return RecordResult{}, c.parseError(resp)
	}

	var result RecordResult
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return RecordResult{}, fmt.Errorf("client: unmarshal record response: %w", err)
	}
	return result, nil
}

// historyReader implements io.ReadCloser for streamed history responses.
type historyReader struct {
	frames   <-chan protocol.Frame
	buf      []byte
	done     bool
	initial  []byte
	usedInit bool
}

func (r *historyReader) Read(p []byte) (int, error) {
	if !r.usedInit && len(r.initial) > 0 {
		r.usedInit = true
		r.buf = r.initial
	}

	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	if r.done {
		return 0, io.EOF
	}

	frame, ok := <-r.frames
	if !ok {
		r.done = true
		return 0, io.EOF
	}

	if frame.Type == protocol.MsgHistoryEnd {
		r.done = true
		return 0, io.EOF
	}

	if frame.Type != protocol.MsgHistory {
		r.done = true
		return 0, fmt.Errorf("client: unexpected frame type during history stream: %s", frame.Type)
	}

	n := copy(p, frame.Payload)
	if n < len(frame.Payload) {
		r.buf = frame.Payload[n:]
	}
	return n, nil
}

func (r *historyReader) Close() error {
	r.done = true
	return nil
}

// History requests scrollback history from the daemon in the specified format.
// Format must be "ansi", "text", or "html". Lines of 0 means all available.
// Returns an io.ReadCloser that streams the history data.
//
// History holds rpcMu for the write, then reads from the history channel.
// The daemon responds with MsgHistory frames (routed to c.history by readControl)
// or MsgError (routed to c.responses).
func (c *Client) History(sessionID, format string, lines int) (io.ReadCloser, error) {
	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Format    string `json:"format"`
		Lines     int    `json:"lines,omitempty"`
	}{
		SessionID: sessionID,
		Format:    format,
		Lines:     lines,
	})
	if err != nil {
		return nil, fmt.Errorf("client: marshal history: %w", err)
	}

	c.rpcMu.Lock()
	err = c.ctrl.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgHistory,
		Payload: payload,
	})
	c.rpcMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("client: write history: %w", err)
	}

	// The daemon responds with MsgHistory/MsgHistoryEnd (→ c.history) or MsgError (→ c.responses).
	select {
	case resp, ok := <-c.history:
		if !ok {
			return nil, fmt.Errorf("client: connection closed")
		}
		if resp.Type == protocol.MsgHistoryEnd {
			return io.NopCloser(strings.NewReader("")), nil
		}
		return &historyReader{
			frames:   c.history,
			initial:  resp.Payload,
			done:     false,
			buf:      nil,
			usedInit: false,
		}, nil
	case resp, ok := <-c.responses:
		if !ok {
			return nil, fmt.Errorf("client: connection closed")
		}
		if resp.Type == protocol.MsgError {
			return nil, c.parseError(resp)
		}
		return nil, fmt.Errorf("client: unexpected response type: %s", resp.Type)
	}
}
