package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// Create sends a create session request to the daemon.
func (c *Client) Create(id string, params CreateParams) (SessionInfo, error) {
	payload, err := json.Marshal(struct {
		ID    string   `json:"id"`
		Shell string   `json:"shell"`
		Args  []string `json:"args,omitempty"`
		Cols  int      `json:"cols"`
		Rows  int      `json:"rows"`
		Cwd   string   `json:"cwd,omitempty"`
		Env   []string `json:"env,omitempty"`
	}{
		ID:    id,
		Shell: params.Shell,
		Args:  params.Args,
		Cols:  params.Cols,
		Rows:  params.Rows,
		Cwd:   params.Cwd,
		Env:   params.Env,
	})
	if err != nil {
		return SessionInfo{}, fmt.Errorf("client: marshal create: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgCreate, payload)
	if err != nil {
		return SessionInfo{}, err
	}

	return c.parseSessionInfo(resp)
}

// Attach attaches to an existing session and returns the snapshot.
func (c *Client) Attach(sessionID string) (AttachResult, error) {
	payload, err := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	if err != nil {
		return AttachResult{}, fmt.Errorf("client: marshal attach: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgAttach, payload)
	if err != nil {
		return AttachResult{}, err
	}

	if resp.Type == protocol.MsgError {
		return AttachResult{}, c.parseError(resp)
	}

	var attachResp struct {
		ID       string `json:"id"`
		State    string `json:"state"`
		Pid      int    `json:"pid"`
		Cols     int    `json:"cols"`
		Rows     int    `json:"rows"`
		Shell    string `json:"shell"`
		Snapshot *struct {
			Replay []byte `json:"replay"`
		} `json:"snapshot,omitempty"`
	}
	if err := json.Unmarshal(resp.Payload, &attachResp); err != nil {
		return AttachResult{}, fmt.Errorf("client: unmarshal attach: %w", err)
	}

	result := AttachResult{
		Snapshot: Snapshot{Replay: nil},
		Session: SessionInfo{
			ID:    attachResp.ID,
			State: attachResp.State,
			Pid:   attachResp.Pid,
			Cols:  attachResp.Cols,
			Rows:  attachResp.Rows,
			Shell: attachResp.Shell,
		},
	}
	if attachResp.Snapshot != nil {
		result.Snapshot = Snapshot{Replay: attachResp.Snapshot.Replay}
	}

	return result, nil
}

// Detach detaches from a session.
func (c *Client) Detach(sessionID string) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	resp, err := c.sendRequest(protocol.MsgDetach, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// Kill terminates a session.
func (c *Client) Kill(sessionID string) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	resp, err := c.sendRequest(protocol.MsgKill, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// Write sends input data to a session's PTY.
func (c *Client) Write(sessionID string, data []byte) error {
	idBytes := []byte(sessionID)
	payload := make([]byte, 0, 1+len(idBytes)+len(data))
	payload = append(payload, byte(len(idBytes)))
	payload = append(payload, idBytes...)
	payload = append(payload, data...)

	resp, err := c.sendRequest(protocol.MsgInput, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// Resize changes the terminal dimensions of a session.
func (c *Client) Resize(sessionID string, cols, rows int) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Cols      int    `json:"cols"`
		Rows      int    `json:"rows"`
	}{SessionID: sessionID, Cols: cols, Rows: rows})

	resp, err := c.sendRequest(protocol.MsgResize, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// UpdateEmulatorScrollback changes the scrollback buffer size of a running
// session. Returns an error if the emulator backend does not support it.
func (c *Client) UpdateEmulatorScrollback(sessionID string, scrollbackLines int) error {
	payload, _ := json.Marshal(struct {
		SessionID       string `json:"session_id"`
		ScrollbackLines int    `json:"scrollback_lines"`
	}{SessionID: sessionID, ScrollbackLines: scrollbackLines})

	resp, err := c.sendRequest(protocol.MsgUpdateEmulatorScrollback, payload)
	if err != nil {
		return fmt.Errorf("update emulator scrollback: %w", err)
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// List returns all sessions, optionally filtered by options.
func (c *Client) List(opts ...ListOption) ([]SessionInfo, error) {
	cfg := &listConfig{prefix: ""}
	for _, opt := range opts {
		opt(cfg)
	}

	var payload []byte
	if cfg.prefix != "" {
		var err error
		payload, err = json.Marshal(struct {
			Prefix string `json:"prefix"`
		}{Prefix: cfg.prefix})
		if err != nil {
			return nil, fmt.Errorf("client: marshal list: %w", err)
		}
	}

	resp, err := c.sendRequest(protocol.MsgList, payload)
	if err != nil {
		return nil, err
	}
	if resp.Type == protocol.MsgError {
		return nil, c.parseError(resp)
	}

	var sessions []SessionInfo
	if err := json.Unmarshal(resp.Payload, &sessions); err != nil {
		return nil, fmt.Errorf("client: unmarshal list: %w", err)
	}
	return sessions, nil
}

// KillPrefix kills all sessions matching the given prefix.
func (c *Client) KillPrefix(prefix string) (KillPrefixResult, error) {
	payload, err := json.Marshal(struct {
		Prefix string `json:"prefix"`
	}{Prefix: prefix})
	if err != nil {
		return KillPrefixResult{}, fmt.Errorf("client: marshal kill_prefix: %w", err)
	}

	resp, err := c.sendRequest(protocol.MsgKillPrefix, payload)
	if err != nil {
		return KillPrefixResult{}, err
	}
	if resp.Type == protocol.MsgError {
		return KillPrefixResult{}, c.parseError(resp)
	}

	var result KillPrefixResult
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return KillPrefixResult{}, fmt.Errorf("client: unmarshal kill_prefix: %w", err)
	}
	return result, nil
}

// Info returns metadata about a specific session.
func (c *Client) Info(sessionID string) (SessionInfo, error) {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	resp, err := c.sendRequest(protocol.MsgInfo, payload)
	if err != nil {
		return SessionInfo{}, err
	}
	return c.parseSessionInfo(resp)
}

func (c *Client) parseSessionInfo(resp protocol.Frame) (SessionInfo, error) {
	if resp.Type == protocol.MsgError {
		return SessionInfo{}, c.parseError(resp)
	}

	var info SessionInfo
	if err := json.Unmarshal(resp.Payload, &info); err != nil {
		return SessionInfo{}, fmt.Errorf("client: unmarshal session: %w", err)
	}
	return info, nil
}

func (c *Client) parseError(resp protocol.Frame) error {
	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Payload, &errResp); err != nil {
		return fmt.Errorf("client: daemon error (unparsable)")
	}
	return fmt.Errorf("client: daemon: %s", errResp.Error)
}
