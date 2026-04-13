package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/config"
	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/pty"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
)

func cmdDaemon(args []string) int {
	// Defaults derived from the global socketPath.
	expandedSocket := expandHome(socketPath)
	baseDir := filepath.Dir(expandedSocket)
	pidFile := filepath.Join(baseDir, "wmux.pid")
	dataDir := filepath.Join(baseDir, "sessions")

	// Parse daemon-specific flags.
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--socket":
			if i+1 < len(args) {
				expandedSocket = expandHome(args[i+1])
				baseDir = filepath.Dir(expandedSocket)
				i++
			}
		case "--pid-file":
			if i+1 < len(args) {
				pidFile = expandHome(args[i+1])
				i++
			}
		case "--data-dir":
			if i+1 < len(args) {
				dataDir = expandHome(args[i+1])
				i++
			}
		case "--log-level":
			// Accepted for compatibility with BuildDaemonArgs but not wired yet.
			if i+1 < len(args) {
				i++
			}
		}
	}

	// Load config (optional — defaults used if file absent).
	configPath := filepath.Join(baseDir, "config.toml")
	cfg := config.Defaults()
	if _, err := os.Stat(configPath); err == nil {
		loaded, err := config.Load(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
			return 1
		}
		cfg = loaded
	}

	// Ensure base directory exists.
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: create base dir: %v\n", err)
		return 1
	}

	// Ensure data directory exists.
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "error: create data dir: %v\n", err)
		return 1
	}

	// Create or load auth token.
	expandedToken := expandHome(tokenPath)
	token, err := auth.Ensure(expandedToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: ensure token: %v\n", err)
		return 1
	}

	// Create IPC listener.
	listener, err := ipc.Listen(expandedSocket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: listen on %s: %v\n", expandedSocket, err)
		return 1
	}

	// Wire dependencies.
	server := transport.NewServer(listener, token)
	spawner := &pty.UnixSpawner{}
	sessionSvc := session.NewService(spawner)
	eventBus := event.NewBus()
	defer eventBus.Close()

	d := daemon.NewDaemon(
		&serverAdapter{srv: server},
		&sessionAdapter{svc: sessionSvc},
		daemon.WithPIDFilePath(pidFile),
		daemon.WithDataDir(dataDir),
		daemon.WithVersion("0.1.0"),
		daemon.WithEventBus(eventBus),
		daemon.WithColdRestore(cfg.History.ColdRestore),
	)

	fmt.Fprintf(os.Stderr, "wmux daemon listening on %s\n", expandedSocket)

	// Handle SIGINT/SIGTERM for graceful shutdown.
	ctx := daemon.HandleSignals(context.Background())

	if err := d.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "wmux daemon stopped")
	return 0
}
