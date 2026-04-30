package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

func TestCmdRepository_Save_NoOpenWriter(t *testing.T) {
	r := newCmdRepository(true)
	// No Open call → Save is a silent no-op.
	r.Save("s1", cmdlifecycle.SessionState{
		History: []cmdlifecycle.Command{{ExitCode: 0}},
	})
}

func TestCmdRepository_OpenSaveClose(t *testing.T) {
	dir := t.TempDir()
	r := newCmdRepository(true)
	path := filepath.Join(dir, "commands.json")
	if err := r.Open("s1", path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	r.Save("s1", cmdlifecycle.SessionState{
		History: []cmdlifecycle.Command{{ExitCode: 7}},
	})
	r.Close("s1")

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := r.peekFile(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("file %s never appeared", path)
}

func TestCmdRepository_DisabledColdRestore(t *testing.T) {
	r := newCmdRepository(false)
	if err := r.Open("s1", "/should/be/ignored"); err != nil {
		t.Errorf("Open should be no-op when disabled, got %v", err)
	}
	r.Save("s1", cmdlifecycle.SessionState{}) // no-op
	r.Close("s1")
}

func TestCmdRepository_CloseUnknown(t *testing.T) {
	r := newCmdRepository(true)
	r.Close("never-opened") // no-op, no panic
}

func TestCmdRepository_CloseAll(t *testing.T) {
	dir := t.TempDir()
	r := newCmdRepository(true)
	for _, sid := range []string{"s1", "s2", "s3"} {
		if err := r.Open(sid, filepath.Join(dir, sid+".json")); err != nil {
			t.Fatalf("Open: %v", err)
		}
	}
	r.CloseAll()
	if len(r.writers) != 0 {
		t.Errorf("writers map = %d, want 0 after CloseAll", len(r.writers))
	}
}

func TestCmdRepository_Save_ImplementsRepository(t *testing.T) {
	var _ cmdlifecycle.Repository = (*cmdRepository)(nil)
}
