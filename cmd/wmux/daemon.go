package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/config"
	"github.com/wblech/wmux/internal/platform/history"
	"github.com/wblech/wmux/pkg/client"
)

func cmdDaemon(args []string) int {
	// Defaults derived from the global socketPath.
	expandedSocket := expandHome(socketPath)
	baseDir := filepath.Dir(expandedSocket)

	var opts []client.Option
	opts = append(opts, client.WithSocket(expandedSocket))
	opts = append(opts, client.WithDataDir(filepath.Join(baseDir, "sessions")))

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
