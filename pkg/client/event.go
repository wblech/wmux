package client

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
