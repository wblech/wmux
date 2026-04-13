package client

import (
	"encoding/json"
	"fmt"

	"github.com/wblech/wmux/internal/platform/protocol"
)

// MetaSet sets a metadata key-value pair on a session.
func (c *Client) MetaSet(sessionID, key, value string) error {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
		Value     string `json:"value"`
	}{SessionID: sessionID, Key: key, Value: value})

	resp, err := c.sendRequest(protocol.MsgMetaSet, payload)
	if err != nil {
		return err
	}
	if resp.Type == protocol.MsgError {
		return c.parseError(resp)
	}
	return nil
}

// MetaGet returns a single metadata value for a session.
func (c *Client) MetaGet(sessionID, key string) (string, error) {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
	}{SessionID: sessionID, Key: key})

	resp, err := c.sendRequest(protocol.MsgMetaGet, payload)
	if err != nil {
		return "", err
	}
	if resp.Type == protocol.MsgError {
		return "", c.parseError(resp)
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return "", fmt.Errorf("client: unmarshal meta get: %w", err)
	}
	return result.Value, nil
}

// MetaGetAll returns all metadata for a session.
func (c *Client) MetaGetAll(sessionID string) (map[string]string, error) {
	payload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Key       string `json:"key"`
	}{SessionID: sessionID, Key: ""})

	resp, err := c.sendRequest(protocol.MsgMetaGet, payload)
	if err != nil {
		return nil, err
	}
	if resp.Type == protocol.MsgError {
		return nil, c.parseError(resp)
	}

	var result struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return nil, fmt.Errorf("client: unmarshal meta get all: %w", err)
	}
	return result.Metadata, nil
}
