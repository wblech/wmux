package daemon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRequest_JSON(t *testing.T) {
	req := ExecRequest{
		SessionID: "my-session",
		Input:     "ls -la\n",
		Newline:   false,
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded ExecRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestExecSyncRequest_JSON(t *testing.T) {
	req := ExecSyncRequest{
		SessionIDs: []string{"s1", "s2"},
		Prefix:     "",
		Input:      "git pull\n",
		Newline:    false,
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded ExecSyncRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestExecSyncRequest_Prefix_JSON(t *testing.T) {
	req := ExecSyncRequest{
		SessionIDs: nil,
		Prefix:     "proj-a",
		Input:      "git pull\n",
		Newline:    false,
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded ExecSyncRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
	assert.Empty(t, decoded.SessionIDs)
}

func TestExecResult_JSON(t *testing.T) {
	result := ExecResult{
		SessionID: "s1",
		OK:        true,
		Error:     "",
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded ExecResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, result, decoded)
}

func TestWaitRequest_ExitMode_JSON(t *testing.T) {
	req := WaitRequest{
		SessionID: "my-session",
		Mode:      "exit",
		Timeout:   5000,
		IdleFor:   0,
		Pattern:   "",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded WaitRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestWaitRequest_IdleMode_JSON(t *testing.T) {
	req := WaitRequest{
		SessionID: "my-session",
		Mode:      "idle",
		Timeout:   10000,
		IdleFor:   2000,
		Pattern:   "",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded WaitRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestWaitRequest_MatchMode_JSON(t *testing.T) {
	req := WaitRequest{
		SessionID: "my-session",
		Mode:      "match",
		Timeout:   0,
		IdleFor:   0,
		Pattern:   "\\$ $",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded WaitRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestWaitResponse_JSON(t *testing.T) {
	ec := 0
	resp := WaitResponse{
		SessionID: "my-session",
		Mode:      "exit",
		ExitCode:  &ec,
		Matched:   false,
		TimedOut:  false,
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded WaitResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, resp, decoded)
}

func TestWaitResponse_Timeout_JSON(t *testing.T) {
	resp := WaitResponse{
		SessionID: "my-session",
		Mode:      "idle",
		ExitCode:  nil,
		Matched:   false,
		TimedOut:  true,
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded WaitResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, resp, decoded)
	assert.Nil(t, decoded.ExitCode)
}

func TestRecordRequest_JSON(t *testing.T) {
	req := RecordRequest{
		SessionID: "my-session",
		Action:    "start",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded RecordRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestRecordRequest_Stop_JSON(t *testing.T) {
	req := RecordRequest{
		SessionID: "my-session",
		Action:    "stop",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded RecordRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "stop", decoded.Action)
}

func TestRecordResponse_JSON(t *testing.T) {
	resp := RecordResponse{
		SessionID: "my-session",
		Recording: true,
		Path:      "/tmp/wmux/my-session/recording-20260413T120000.cast",
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded RecordResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, resp, decoded)
}

func TestHistoryRequest_JSON(t *testing.T) {
	req := HistoryRequest{
		SessionID: "my-session",
		Format:    "html",
		Lines:     500,
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded HistoryRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, req, decoded)
}

func TestHistoryRequest_Defaults_JSON(t *testing.T) {
	req := HistoryRequest{
		SessionID: "s1",
		Format:    "ansi",
		Lines:     0,
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded HistoryRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "ansi", decoded.Format)
	assert.Equal(t, 0, decoded.Lines)
}

func TestSentinelErrors(t *testing.T) {
	assert.NotEqual(t, ErrDaemonRunning.Error(), ErrDaemonNotRunning.Error())
	assert.NotEqual(t, ErrAlreadyAttached.Error(), ErrNotAttached.Error())
	assert.NotEqual(t, ErrSessionNotSpecified.Error(), ErrAlreadyAttached.Error())
}
