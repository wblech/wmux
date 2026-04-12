package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
