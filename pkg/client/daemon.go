package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/event"
	"github.com/wblech/wmux/internal/platform/ipc"
	"github.com/wblech/wmux/internal/platform/pty"
	"github.com/wblech/wmux/internal/session"
	"github.com/wblech/wmux/internal/transport"
)

// Daemon is an embeddable wmux daemon that integrators can run in-process
// or as a background service.
type Daemon struct {
	cfg      *config
	eventBus *event.Bus
}

// NewDaemon creates a daemon configured from the given options.
// Call Serve to start listening for client connections.
func NewDaemon(opts ...Option) (*Daemon, error) {
	cfg := newConfig(opts...)
	resolveConfig(cfg)

	nsDir := filepath.Dir(cfg.socket)
	if err := os.MkdirAll(nsDir, 0o700); err != nil {
		return nil, fmt.Errorf("client: create namespace dir: %w", err)
	}

	if err := os.MkdirAll(cfg.dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("client: create data dir: %w", err)
	}

	return &Daemon{
		cfg:      cfg,
		eventBus: event.NewBus(),
	}, nil
}

// Serve starts the daemon, listening for client connections on the configured
// socket. Blocks until ctx is cancelled. Returns nil on clean shutdown.
func (d *Daemon) Serve(ctx context.Context) error {
	defer d.eventBus.Close()

	token, err := auth.Ensure(d.cfg.tokenPath)
	if err != nil {
		return fmt.Errorf("client: ensure token: %w", err)
	}

	listener, err := ipc.Listen(d.cfg.socket)
	if err != nil {
		return fmt.Errorf("client: listen: %w", err)
	}

	server := transport.NewServer(listener, token)
	spawner := &pty.UnixSpawner{}
	sessionSvc := session.NewService(spawner)

	pidFile := filepath.Join(filepath.Dir(d.cfg.socket), "wmux.pid")

	dd := daemon.NewDaemon(
		&serverAdapter{srv: server},
		&sessionAdapter{svc: sessionSvc},
		daemon.WithPIDFilePath(pidFile),
		daemon.WithDataDir(d.cfg.dataDir),
		daemon.WithVersion("0.1.0"),
		daemon.WithEventBus(d.eventBus),
		daemon.WithColdRestore(d.cfg.coldRestore),
		daemon.WithMaxScrollbackSize(d.cfg.maxScrollbackSize),
	)

	if err := dd.Start(ctx); err != nil {
		return fmt.Errorf("client: daemon start: %w", err)
	}

	return nil
}

const daemonSentinel = "__wmux_daemon__"

// isDaemonMode checks if the args contain the daemon sentinel.
func isDaemonMode(args []string) bool {
	for _, a := range args {
		if a == daemonSentinel {
			return true
		}
	}
	return false
}

// ServeDaemon checks if the current process was invoked in daemon mode
// (by a prior client.New auto-start). If so, it runs the daemon and
// returns true. If not, returns false immediately.
//
// Integrators add this as the first line in main():
//
//	func main() {
//	    if client.ServeDaemon(os.Args) {
//	        return
//	    }
//	    // normal app...
//	}
func ServeDaemon(args []string) bool {
	if !isDaemonMode(args) {
		return false
	}

	opts := parseDaemonArgs(args)
	d, err := NewDaemon(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux daemon: %v\n", err)
		os.Exit(1)
	}

	ctx := daemon.HandleSignals(context.Background())
	if err := d.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "wmux daemon: %v\n", err)
		os.Exit(1)
	}

	return true
}

// parseDaemonArgs extracts options from the sentinel-style args.
func parseDaemonArgs(args []string) []Option {
	var opts []Option
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--base-dir":
			if i+1 < len(args) {
				opts = append(opts, WithBaseDir(args[i+1]))
				i++
			}
		case "--namespace":
			if i+1 < len(args) {
				opts = append(opts, WithNamespace(args[i+1]))
				i++
			}
		case "--socket":
			if i+1 < len(args) {
				opts = append(opts, WithSocket(args[i+1]))
				i++
			}
		case "--data-dir":
			if i+1 < len(args) {
				opts = append(opts, WithDataDir(args[i+1]))
				i++
			}
		}
	}
	return opts
}
