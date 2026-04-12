package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// HandleSignals returns a context that is canceled when SIGTERM or SIGINT
// is received. The returned context is also canceled if the parent ctx
// is canceled.
func HandleSignals(ctx context.Context) context.Context {
	sigCtx, cancel := context.WithCancel(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		defer signal.Stop(sigCh)

		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
			cancel()
		}
	}()

	return sigCtx
}

// GracefulShutdown kills all managed sessions and stops the daemon.
func (d *Daemon) GracefulShutdown() {
	sessions := d.sessionSvc.List()
	for _, s := range sessions {
		_ = d.sessionSvc.Kill(s.ID)
	}

	d.Stop()
}
