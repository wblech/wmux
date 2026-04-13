package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// ForwardEnv sends environment variables to the daemon for a session.
// The daemon creates stable symlinks for socket/file paths and writes
// an env file for other values.
func (c *Client) ForwardEnv(sessionID string, env map[string]string) error {
	payload, err := json.Marshal(struct {
		SessionID string            `json:"session_id"`
		Env       map[string]string `json:"env"`
	}{SessionID: sessionID, Env: env})
	if err != nil {
		return fmt.Errorf("client: marshal env forward: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgEnvForward, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}
