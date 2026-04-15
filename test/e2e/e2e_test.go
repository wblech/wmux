package e2e

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestE2E_SessionCreatedEvent(t *testing.T) {
	env := startTestDaemon(t)
	c := connectTestClient(t, env)

	received := make(chan client.Event, 1)
	c.OnEvent(func(evt client.Event) {
		if evt.Type == "session.created" {
			received <- evt
		}
	})

	_, err := c.Create("e2e-sess", client.CreateParams{
		Shell: "/bin/sh",
		Args:  []string{"-c", "exit 0"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)

	select {
	case evt := <-received:
		assert.Equal(t, "session.created", evt.Type)
		assert.Equal(t, "e2e-sess", evt.SessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session.created event")
	}
}

func TestE2E_SessionExitedEvent(t *testing.T) {
	env := startTestDaemon(t)
	c := connectTestClient(t, env)

	received := make(chan client.Event, 1)
	c.OnEvent(func(evt client.Event) {
		if evt.Type == "session.exited" {
			received <- evt
		}
	})

	_, err := c.Create("exit-sess", client.CreateParams{
		Shell: "/bin/sh",
		Args:  []string{"-c", "exit 0"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)

	select {
	case evt := <-received:
		assert.Equal(t, "session.exited", evt.Type)
		assert.Equal(t, "exit-sess", evt.SessionID)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for session.exited event — this is the original bug")
	}
}

func TestE2E_KillSessionEvent(t *testing.T) {
	env := startTestDaemon(t)
	c := connectTestClient(t, env)

	received := make(chan client.Event, 4)
	c.OnEvent(func(evt client.Event) {
		received <- evt
	})

	_, err := c.Create("kill-sess", client.CreateParams{
		Shell: "/bin/sh",
		Args:  []string{"-c", "sleep 60"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)

	// Drain the created event.
	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session.created")
	}

	require.NoError(t, c.Kill("kill-sess"))

	select {
	case evt := <-received:
		assert.Equal(t, "session.exited", evt.Type)
		assert.Equal(t, "kill-sess", evt.SessionID)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for kill event")
	}
}

func TestE2E_TwoClientsReceiveEvents(t *testing.T) {
	env := startTestDaemon(t)
	c1 := connectTestClient(t, env)
	c2 := connectTestClient(t, env)

	received1 := make(chan client.Event, 1)
	received2 := make(chan client.Event, 1)

	c1.OnEvent(func(evt client.Event) {
		if evt.Type == "session.created" {
			received1 <- evt
		}
	})
	c2.OnEvent(func(evt client.Event) {
		if evt.Type == "session.created" {
			received2 <- evt
		}
	})

	_, err := c1.Create("multi-sess", client.CreateParams{
		Shell: "/bin/sh",
		Args:  []string{"-c", "exit 0"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)

	for _, ch := range []chan client.Event{received1, received2} {
		select {
		case evt := <-ch:
			assert.Equal(t, "session.created", evt.Type)
			assert.Equal(t, "multi-sess", evt.SessionID)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out — one client did not receive the event")
		}
	}
}

func TestE2E_ClientDisconnectCleanup(t *testing.T) {
	env := startTestDaemon(t)

	c1, err := client.New(
		client.WithSocket(env.SocketPath),
		client.WithTokenPath(env.TokenPath),
		client.WithAutoStart(false),
	)
	require.NoError(t, err)
	c1.OnEvent(func(_ client.Event) {})
	require.NoError(t, c1.Close())

	time.Sleep(50 * time.Millisecond)

	c2 := connectTestClient(t, env)

	received := make(chan client.Event, 1)
	c2.OnEvent(func(evt client.Event) {
		if evt.Type == "session.created" {
			received <- evt
		}
	})

	_, err = c2.Create("cleanup-sess", client.CreateParams{
		Shell: "/bin/sh",
		Args:  []string{"-c", "exit 0"},
		Cols:  80,
		Rows:  24,
		Cwd:   "",
		Env:   nil,
	})
	require.NoError(t, err)

	select {
	case evt := <-received:
		assert.Equal(t, "session.created", evt.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("daemon broken after client disconnect")
	}
}
