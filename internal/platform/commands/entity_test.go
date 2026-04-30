package commands

import (
	"encoding/json"
	"strings"
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
	if strings.Contains(string(data), "in_flight") {
		t.Errorf("expected omitempty: %s", data)
	}
}
