package e2e

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/pty"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
	"github.com/wblech/wmux/pkg/client"
)

// testDaemonEnv holds the running daemon and its connection metadata.
type testDaemonEnv struct {
	SocketPath string
	TokenPath  string
}

// startTestDaemon creates a real daemon with event bus, starts it, and returns
// the socket/token paths. Cleanup is registered via t.Cleanup.
func startTestDaemon(t *testing.T) *testDaemonEnv {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	dir, err := os.MkdirTemp("", "wmux-e2e")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sock := filepath.Join(dir, "d.sock")
	tokenPath := filepath.Join(dir, "d.token")

	token, err := auth.Generate()
	require.NoError(t, err)
	require.NoError(t, auth.SaveToFile(token, tokenPath))

	ln, err := ipc.Listen(sock)
	require.NoError(t, err)

	srv := transport.NewServer(ln, token)
	spawner := &pty.UnixSpawner{}
	svc := session.NewService(spawner)
	bus := event.NewBus()

	d := daemon.NewDaemon(
		&serverAdapter{srv: srv},
		&sessionAdapter{svc: svc},
		daemon.WithEventBus(bus),
	)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = d.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	t.Cleanup(func() {
		cancel()
		bus.Close()
	})

	return &testDaemonEnv{
		SocketPath: sock,
		TokenPath:  tokenPath,
	}
}

// connectTestClient creates a real client connected to the test daemon.
func connectTestClient(t *testing.T, env *testDaemonEnv) *client.Client {
	t.Helper()

	c, err := client.New(
		client.WithSocket(env.SocketPath),
		client.WithTokenPath(env.TokenPath),
		client.WithAutoStart(false),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	return c
}
