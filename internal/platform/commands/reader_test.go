package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileNotExist(t *testing.T) {
	got, err := Load("/nonexistent/path/commands.json")
	if err != nil {
		t.Errorf("err = %v, want nil for missing file", err)
	}
	if got.Version != 0 {
		t.Errorf("Version = %d, want 0 for missing file", got.Version)
	}
	if got.SessionID != "" {
		t.Errorf("SessionID = %q, want empty for missing file", got.SessionID)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{
		"version": 1,
		"session_id": "s1",
		"saved_at": "2026-04-30T12:00:00Z",
		"history": [{"started_at":"2026-04-30T12:00:00Z","ended_at":"2026-04-30T12:00:01Z","exit_code":0,"start_row":1,"end_row":2}]
	}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID = %q, want s1", got.SessionID)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1", len(got.History))
	}
}

func TestLoad_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for corrupted JSON")
	}
}

func TestLoad_UnsupportedFutureVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{"version": 2, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("err = %v, want ErrUnsupportedVersion", err)
	}
}

func TestLoad_InFlightAbsent_TreatedAsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{"version": 1, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z", "history": []}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if got.InFlight != nil {
		t.Error("absent in_flight must Unmarshal as nil")
	}
}

func TestLoad_InFlightExplicitNull_TreatedAsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	contents := `{"version": 1, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z", "in_flight": null}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(path)
	if got.InFlight != nil {
		t.Error("explicit in_flight: null must Unmarshal as nil")
	}
}
