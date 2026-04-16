package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/config"
	"github.com/wblech/wmux/internal/platform/history"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/pkg/client"
)

// cliEmulatorFactory wraps AddonManager to implement client.EmulatorFactory.
type cliEmulatorFactory struct {
	mgr *session.AddonManager
}

func (f *cliEmulatorFactory) Create(sessionID string, cols, rows int) client.ScreenEmulator {
	return &cliEmulatorAdapter{em: f.mgr.EmulatorFor(sessionID, cols, rows)}
}

func (f *cliEmulatorFactory) Close() {
	f.mgr.Shutdown()
}

// cliEmulatorAdapter wraps session.ScreenEmulator to implement client.ScreenEmulator.
type cliEmulatorAdapter struct {
	em session.ScreenEmulator
}

func (a *cliEmulatorAdapter) Process(data []byte) { a.em.Process(data) }
func (a *cliEmulatorAdapter) Snapshot() client.Snapshot {
	snap := a.em.Snapshot()
	return client.Snapshot{
		Scrollback: snap.Scrollback,
		Viewport:   snap.Viewport,
	}
}
func (a *cliEmulatorAdapter) Resize(cols, rows int) { a.em.Resize(cols, rows) }

func cmdDaemon(args []string) int {
	// Defaults derived from the global socketPath.
	expandedSocket := expandHome(socketPath)
	baseDir := filepath.Dir(expandedSocket)

	var opts []client.Option
	opts = append(opts, client.WithSocket(expandedSocket))
	opts = append(opts, client.WithDataDir(filepath.Join(baseDir, "sessions")))

	var xtermBinPath string

	// Parse daemon-specific flags.
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--socket":
			if i+1 < len(args) {
				expandedSocket = expandHome(args[i+1])
				baseDir = filepath.Dir(expandedSocket)
				opts = append(opts, client.WithSocket(expandedSocket))
				opts = append(opts, client.WithDataDir(filepath.Join(baseDir, "sessions")))
				i++
			}
		case "--data-dir":
			if i+1 < len(args) {
				opts = append(opts, client.WithDataDir(expandHome(args[i+1])))
				i++
			}
		case "--log-level":
			if i+1 < len(args) {
				i++
			}
		case "--xterm-bin":
			if i+1 < len(args) {
				xtermBinPath = args[i+1]
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

	maxScrollback, err := history.ParseSize(cfg.History.MaxPerSession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parse max_per_session: %v\n", err)
		return 1
	}

	opts = append(opts,
		client.WithColdRestore(cfg.History.ColdRestore),
		client.WithMaxScrollbackSize(maxScrollback),
	)

	if xtermBinPath != "" {
		starter := session.NewCommandProcessStarter("node", xtermBinPath)
		mgr := session.NewAddonManager(starter)
		opts = append(opts, client.WithEmulatorFactory(&cliEmulatorFactory{mgr: mgr}))
	}

	d, err := client.NewDaemon(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "wmux daemon listening on %s\n", expandedSocket)

	ctx := daemon.HandleSignals(context.Background())

	if err := d.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "wmux daemon stopped")
	return 0
}
