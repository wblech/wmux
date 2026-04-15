package client

import "github.com/wblech/wmux/internal/platform/protocol"

// OnData registers a callback for PTY output data.
// The callback receives the session ID and raw output bytes.
func (c *Client) OnData(handler func(sessionID string, data []byte)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dataHandler = handler
}

// OnEvent registers a callback for daemon events.
func (c *Client) OnEvent(handler func(Event)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evtHandler = handler
}

// subscribe sends a MsgEvent frame to the daemon to activate event forwarding.
func (c *Client) subscribe() error {
	resp, err := c.sendRequest(protocol.MsgEvent, nil)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}
