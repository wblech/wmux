package cmdlifecycle

import (
	"testing"
	"time"
)

func TestState_String(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateIdle, "idle"},
		{StatePromptShown, "prompt_shown"},
		{StateUserTyping, "user_typing"},
		{StateCommandRunning, "command_running"},
		{State(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("State(%d).String() = %q, want %q", c.state, got, c.want)
		}
	}
}

func TestCommand_ZeroValueOrphanFalse(t *testing.T) {
	var c Command
	if c.Orphan {
		t.Error("zero-value Command should have Orphan=false")
	}
	if c.OrphanReason != "" {
		t.Error("zero-value Command should have empty OrphanReason")
	}
}

func TestSessionState_ZeroValue(t *testing.T) {
	var s SessionState
	if s.History != nil {
		t.Error("zero-value SessionState should have nil History")
	}
	if s.InFlight != nil {
		t.Error("zero-value SessionState should have nil InFlight")
	}
}

func TestOrphanReasonConsts(t *testing.T) {
	if OrphanReasonMissingD != "missing_d_marker" {
		t.Errorf("OrphanReasonMissingD = %q, want %q", OrphanReasonMissingD, "missing_d_marker")
	}
	if OrphanReasonDaemonRestart != "daemon_restart" {
		t.Errorf("OrphanReasonDaemonRestart = %q, want %q", OrphanReasonDaemonRestart, "daemon_restart")
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrSessionNotRegistered == nil {
		t.Error("ErrSessionNotRegistered should be non-nil")
	}
	if ErrSessionExists == nil {
		t.Error("ErrSessionExists should be non-nil")
	}
	if ErrInvalidOSCCode == nil {
		t.Error("ErrInvalidOSCCode should be non-nil")
	}
}

func TestCommand_RowMonotonicInvariant(t *testing.T) {
	c := Command{StartRow: 100, EndRow: 100}
	if c.EndRow < c.StartRow {
		t.Error("EndRow must be >= StartRow")
	}
	c.EndRow = 200
	if c.EndRow < c.StartRow {
		t.Error("EndRow must be >= StartRow")
	}
}

func TestCommand_TimestampZero(t *testing.T) {
	c := Command{StartedAt: time.Now()}
	if c.EndedAt.IsZero() != true {
		t.Error("zero-value EndedAt should be IsZero")
	}
}
