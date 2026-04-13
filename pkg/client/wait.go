package client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// WaitResult holds the outcome of a wait operation.
type WaitResult struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"`
	ExitCode  *int   `json:"exit_code"`
	Matched   bool   `json:"matched"`
	TimedOut  bool   `json:"timed_out"`
}

type waitConfig struct {
	timeout time.Duration
}

// WaitOption configures a wait call.
type WaitOption func(*waitConfig)

// WithTimeout sets the maximum time to wait before returning a timeout result.
// Zero means no timeout (wait indefinitely).
func WithTimeout(d time.Duration) WaitOption {
	return func(c *waitConfig) {
		c.timeout = d
	}
}

// UntilExit waits until the session exits.
func (c *Client) UntilExit(sessionID string, opts ...WaitOption) (*WaitResult, error) {
	cfg := &waitConfig{timeout: 0}
	for _, opt := range opts {
		opt(cfg)
	}

	return c.wait(sessionID, "exit", cfg.timeout, 0, "")
}

// UntilIdle waits until the session has no output for the specified duration.
func (c *Client) UntilIdle(sessionID string, d time.Duration, opts ...WaitOption) (*WaitResult, error) {
	cfg := &waitConfig{timeout: 0}
	for _, opt := range opts {
		opt(cfg)
	}

	return c.wait(sessionID, "idle", cfg.timeout, d, "")
}

// UntilMatch waits until the given pattern appears in the session output.
func (c *Client) UntilMatch(sessionID string, pattern string, opts ...WaitOption) (*WaitResult, error) {
	cfg := &waitConfig{timeout: 0}
	for _, opt := range opts {
		opt(cfg)
	}

	return c.wait(sessionID, "match", cfg.timeout, 0, pattern)
}

// wait is the private implementation that sends the MsgWait request.
func (c *Client) wait(sessionID, mode string, timeout, idleFor time.Duration, pattern string) (*WaitResult, error) {
	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Mode      string `json:"mode"`
		Timeout   int64  `json:"timeout"`
		IdleFor   int64  `json:"idle_for,omitempty"`
		Pattern   string `json:"pattern,omitempty"`
	}{
		SessionID: sessionID,
		Mode:      mode,
		Timeout:   timeout.Milliseconds(),
		IdleFor:   idleFor.Milliseconds(),
		Pattern:   pattern,
	})
	if err != nil {
		return nil, fmt.Errorf("client: marshal wait: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgWait, payload)
	if err != nil {
		return nil, err
	}
	if resp.Type == protocol.MsgError {
		return nil, c.parseError(resp)
	}

	var result WaitResult
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return nil, fmt.Errorf("client: unmarshal wait response: %w", err)
	}

	return &result, nil
}
