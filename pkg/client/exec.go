package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// ExecResult represents the outcome of an exec operation on a single session.
type ExecResult struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

type execConfig struct {
	newline bool
}

// ExecOption configures an Exec call.
type ExecOption func(*execConfig)

// WithNewline controls whether a newline is appended to the input. Default: true.
func WithNewline(enabled bool) ExecOption {
	return func(c *execConfig) {
		c.newline = enabled
	}
}

// Exec sends input to a single session without attaching.
func (c *Client) Exec(sessionID, input string, opts ...ExecOption) error {
	cfg := &execConfig{newline: true}
	for _, opt := range opts {
		opt(cfg)
	}

	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Input     string `json:"input"`
		Newline   bool   `json:"newline"`
	}{
		SessionID: sessionID,
		Input:     input,
		Newline:   cfg.newline,
	})
	if err != nil {
		return fmt.Errorf("client: marshal exec: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgExec, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// ExecSync sends input to multiple sessions specified by ID. Best-effort.
func (c *Client) ExecSync(input string, targets ...string) ([]ExecResult, error) {
	payload, err := json.Marshal(struct {
		SessionIDs []string `json:"session_ids"`
		Input      string   `json:"input"`
		Newline    bool     `json:"newline"`
	}{
		SessionIDs: targets,
		Input:      input,
		Newline:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("client: marshal exec_sync: %w", err)
	}

	return c.execSyncRequest(payload)
}

// ExecPrefix sends input to all sessions matching the prefix. Best-effort.
func (c *Client) ExecPrefix(prefix, input string) ([]ExecResult, error) {
	payload, err := json.Marshal(struct {
		Prefix  string `json:"prefix"`
		Input   string `json:"input"`
		Newline bool   `json:"newline"`
	}{
		Prefix:  prefix,
		Input:   input,
		Newline: true,
	})
	if err != nil {
		return nil, fmt.Errorf("client: marshal exec_prefix: %w", err)
	}

	return c.execSyncRequest(payload)
}

func (c *Client) execSyncRequest(payload []byte) ([]ExecResult, error) {
	resp, err := c.sendRequest(protocol.MsgExecSync, payload)
	if err != nil {
		return nil, err
	}
	if resp.Type == protocol.MsgError {
		return nil, c.parseError(resp)
	}

	var syncResp struct {
		Results []ExecResult `json:"results"`
	}
	if err := json.Unmarshal(resp.Payload, &syncResp); err != nil {
		return nil, fmt.Errorf("client: unmarshal exec_sync response: %w", err)
	}
	return syncResp.Results, nil
}
