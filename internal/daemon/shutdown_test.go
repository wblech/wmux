package daemon

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalHandler_CancelsContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal handling not supported on Windows")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCtx := HandleSignals(ctx)

	proc, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)

	err = proc.Signal(syscall.SIGINT)
	require.NoError(t, err)

	select {
	case <-sigCtx.Done():
		// OK — signal canceled the context.
	case <-time.After(2 * time.Second):
		t.Fatal("signal handler did not cancel context")
	}
}

func TestSignalHandler_ParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCtx := HandleSignals(ctx)

	cancel()

	select {
	case <-sigCtx.Done():
		// OK — parent cancel propagated.
	case <-time.After(time.Second):
		t.Fatal("derived context not canceled when parent canceled")
	}
}

func TestGracefulShutdown_KillsSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	d, _, _ := testDaemon(t)
	cancel := startDaemon(t, d)
	defer cancel()

	_, err := d.sessionSvc.Create("shutdown-test", SessionCreateOptions{
		Shell: "/bin/sh",
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)

	sessions := d.sessionSvc.List()
	assert.Len(t, sessions, 1)

	d.GracefulShutdown()

	sessions = d.sessionSvc.List()
	assert.Empty(t, sessions)
}
