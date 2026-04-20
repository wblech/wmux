package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wblech/wmux/internal/daemon"
	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/debug"
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

	// Create debug tracer if configured (programmatic options or env vars).
	var tracer *debug.Tracer
	debugCfg := d.resolveDebugConfig()
	if debugCfg.Enabled {
		var tracerOpts []debug.TracerOption
		if debugCfg.MaxSizeMB > 0 {
			tracerOpts = append(tracerOpts, debug.WithMaxSize(debugCfg.MaxSizeMB))
		}
		if debugCfg.MaxFiles > 0 {
			tracerOpts = append(tracerOpts, debug.WithMaxFiles(debugCfg.MaxFiles))
		}
		var tracerErr error
		tracer, tracerErr = debug.NewTracer(debugCfg.Path, debugCfg.Level, tracerOpts...)
		if tracerErr != nil {
			return fmt.Errorf("client: create debug tracer: %w", tracerErr)
		}
		defer tracer.Close() //nolint:errcheck
	}

	var sessionOpts []session.Option
	if d.cfg.emulatorFactory != nil {
		sessionOpts = append(sessionOpts, session.WithEmulatorFactory(
			&emulatorFactoryAdapter{f: d.cfg.emulatorFactory},
		))
	}
	sessionOpts = append(sessionOpts, session.WithTracer(tracer))
	sessionSvc := session.NewService(spawner, sessionOpts...)

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
		daemon.WithTracer(tracer),
	)

	if err := dd.Start(ctx); err != nil {
		return fmt.Errorf("client: daemon start: %w", err)
	}

	return nil
}

// resolveDebugConfig merges programmatic options with env var fallbacks.
// Programmatic options take precedence.
func (d *Daemon) resolveDebugConfig() debug.EnvConfig {
	env := debug.ReadEnv()

	cfg := debug.EnvConfig{ //nolint:exhaustruct
		MaxSizeMB: 50,
		MaxFiles:  7,
	}

	// Programmatic options take precedence.
	if d.cfg.debugLogPath != "" {
		cfg.Enabled = true
		cfg.Path = d.cfg.debugLogPath
		cfg.Level = debug.LevelChunk
	} else if env.Enabled {
		cfg.Enabled = true
		cfg.Path = env.Path
		cfg.Level = env.Level
	}

	if !cfg.Enabled {
		return cfg
	}

	if d.cfg.debugLevel > 0 {
		cfg.Level = debug.ClampLevel(d.cfg.debugLevel)
	} else if env.Level > debug.LevelOff {
		cfg.Level = env.Level
	}

	if d.cfg.debugMaxSize > 0 {
		cfg.MaxSizeMB = d.cfg.debugMaxSize
	} else if env.MaxSizeMB > 0 {
		cfg.MaxSizeMB = env.MaxSizeMB
	}

	if d.cfg.debugMaxFiles > 0 {
		cfg.MaxFiles = d.cfg.debugMaxFiles
	} else if env.MaxFiles > 0 {
		cfg.MaxFiles = env.MaxFiles
	}

	return cfg
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
// Extra options are merged after CLI-parsed flags, allowing integrators
// to inject non-serializable configuration such as emulator factories:
//
//	func main() {
//	    if handled, err := client.ServeDaemon(os.Args, charmvt.Backend()); handled {
//	        if err != nil {
//	            log.Fatal(err)
//	        }
//	        return
//	    }
//	    // normal app...
//	}
func ServeDaemon(args []string, extraOpts ...Option) (bool, error) {
	if !isDaemonMode(args) {
		return false, nil
	}

	opts := parseDaemonArgs(args)
	opts = append(opts, extraOpts...)
	d, err := NewDaemon(opts...)
	if err != nil {
		return true, fmt.Errorf("wmux daemon: %w", err)
	}

	ctx := daemon.HandleSignals(context.Background())
	if err := d.Serve(ctx); err != nil {
		return true, fmt.Errorf("wmux daemon: %w", err)
	}

	return true, nil
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
		case "--token-path":
			if i+1 < len(args) {
				opts = append(opts, WithTokenPath(args[i+1]))
				i++
			}
		case "--data-dir":
			if i+1 < len(args) {
				opts = append(opts, WithDataDir(args[i+1]))
				i++
			}
		case "--cold-restore":
			opts = append(opts, WithColdRestore(true))
		case "--max-scrollback":
			if i+1 < len(args) {
				n, err := strconv.ParseInt(args[i+1], 10, 64)
				if err == nil {
					opts = append(opts, WithMaxScrollbackSize(n))
				}
				i++
			}
		}
	}
	return opts
}
