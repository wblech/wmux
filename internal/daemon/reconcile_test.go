package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wblech/wmux/internal/cmdlifecycle"
	"github.com/wblech/wmux/internal/platform/history"
)

func TestReconcileOrphans_UpdatesUnclean(t *testing.T) {
	base := t.TempDir()

	// Session with missing endedAt (unclean shutdown).
	d1 := filepath.Join(base, "orphan-1")
	require.NoError(t, os.MkdirAll(d1, 0755))
	require.NoError(t, history.WriteMetadata(d1, history.Metadata{
		SessionID: "orphan-1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now().Add(-time.Hour),
		EndedAt:   nil,
		ExitCode:  nil,
	}))

	// Clean session (has endedAt).
	d2 := filepath.Join(base, "clean-1")
	require.NoError(t, os.MkdirAll(d2, 0755))
	end := time.Now().Add(-30 * time.Minute)
	require.NoError(t, history.WriteMetadata(d2, history.Metadata{
		SessionID: "clean-1",
		Shell:     "/bin/sh",
		Cwd:       "/home",
		Cols:      80,
		Rows:      24,
		StartedAt: time.Now().Add(-time.Hour),
		EndedAt:   &end,
		ExitCode:  intPtr(0),
	}))

	count, err := ReconcileOrphans(base)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify orphan was updated.
	meta, err := history.ReadMetadata(d1)
	require.NoError(t, err)
	assert.NotNil(t, meta.EndedAt)
	assert.NotNil(t, meta.ExitCode)
	assert.Equal(t, -1, *meta.ExitCode)

	// Verify clean session was not touched.
	meta2, err := history.ReadMetadata(d2)
	require.NoError(t, err)
	assert.Equal(t, 0, *meta2.ExitCode)
}

func TestReconcileOrphans_EmptyDir(t *testing.T) {
	base := t.TempDir()

	count, err := ReconcileOrphans(base)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestReconcileOrphans_NonExistentDir(t *testing.T) {
	count, err := ReconcileOrphans("/nonexistent/path")
	require.Error(t, err)
	assert.Equal(t, 0, count)
}

func intPtr(n int) *int {
	return &n
}

func TestRestoreCommandsForSession_LoadsHistory(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contents := fmt.Sprintf(`{
		"version": 1,
		"session_id": "s1",
		"saved_at": "%s",
		"history": [{"started_at":"2026-04-30T12:00:00Z","ended_at":"2026-04-30T12:00:01Z","exit_code":0,"start_row":1,"end_row":2}]
	}`, now.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(sessDir, "commands.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)

	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Fatalf("RestoreCommandsForSession: %v", err)
	}

	state, err := cmdSvc.Snapshot("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 1 {
		t.Errorf("History len = %d, want 1", len(state.History))
	}
}

func TestRestoreCommandsForSession_HandlesMissingFile(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	_ = os.MkdirAll(sessDir, 0o755)

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)

	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if _, err := cmdSvc.Snapshot("s1"); !errors.Is(err, cmdlifecycle.ErrSessionNotRegistered) {
		t.Errorf("session should not be registered when file missing")
	}
}

func TestRestoreCommandsForSession_FutureVersion_Skipped(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	_ = os.MkdirAll(sessDir, 0o755)
	contents := `{"version": 99, "session_id": "s1", "saved_at": "2026-04-30T12:00:00Z"}`
	if err := os.WriteFile(filepath.Join(sessDir, "commands.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)
	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Errorf("future version should be skipped, got %v", err)
	}
	if _, err := cmdSvc.Snapshot("s1"); !errors.Is(err, cmdlifecycle.ErrSessionNotRegistered) {
		t.Error("session should not be registered after future-version file")
	}
}

func TestRestoreCommandsForSession_InFlightBecomesOrphan(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "s1")
	_ = os.MkdirAll(sessDir, 0o755)
	contents := `{
		"version": 1,
		"session_id": "s1",
		"saved_at": "2026-04-30T12:00:00Z",
		"history": [],
		"in_flight": {"started_at":"2026-04-30T12:00:00Z","start_row":42}
	}`
	if err := os.WriteFile(filepath.Join(sessDir, "commands.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	repo := newCmdRepository(true)
	cmdSvc := cmdlifecycle.NewService(repo)
	if err := RestoreCommandsForSession(dir, "s1", cmdSvc); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	state, _ := cmdSvc.Snapshot("s1")
	if len(state.History) != 1 {
		t.Fatalf("InFlight should become orphan history entry; got %d", len(state.History))
	}
	if state.InFlight != nil {
		t.Error("after Restore, InFlight must be nil")
	}
	if state.History[0].OrphanReason != cmdlifecycle.OrphanReasonDaemonRestart {
		t.Errorf("OrphanReason = %q, want daemon_restart", state.History[0].OrphanReason)
	}
}
