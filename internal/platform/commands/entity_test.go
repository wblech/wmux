package commands

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func TestFile_RoundtripJSON(t *testing.T) {
	when := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	f := File{
		Version:   CurrentVersion,
		SessionID: "s1",
		SavedAt:   when,
		History: []cmdlifecycle.Command{{
			StartedAt: when,
			EndedAt:   when.Add(time.Second),
			ExitCode:  0,
			StartRow:  10,
			EndRow:    20,
		}},
		InFlight: nil,
	}

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	var got File
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, CurrentVersion)
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want s1", got.SessionID)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1", len(got.History))
	}
}

func TestFile_OmitInFlight_WhenNil(t *testing.T) {
	f := File{Version: 1, SessionID: "s1", SavedAt: time.Now()}
	data, _ := json.Marshal(f)
	if string(data) == "" {
		t.Fatal("empty marshal")
	}
	if contains(string(data), "in_flight") {
		t.Errorf("expected omitempty: %s", data)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSentinelErrors(t *testing.T) {
	if ErrUnsupportedVersion == nil {
		t.Error("ErrUnsupportedVersion must be non-nil")
	}
	if ErrWriterClosed == nil {
		t.Error("ErrWriterClosed must be non-nil")
	}
}

func TestCurrentVersion(t *testing.T) {
	if CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want 1 (initial schema)", CurrentVersion)
	}
}
