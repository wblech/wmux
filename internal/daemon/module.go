package daemon

import (
	"go.uber.org/fx"

	"github.com/wblech/wmux/internal/cmdlifecycle"
)

// Module provides the daemon to the fx dependency injection container.
//
// Includes cmdlifecycle.Module so cmdlifecycle.Service is provided when the
// daemon's cmdRepo (constructed via WithColdRestore) is supplied as the
// cmdlifecycle.Repository.
var Module = fx.Options(
	fx.Provide(NewDaemon),
	cmdlifecycle.Module,
	fx.Provide(func(d *Daemon) cmdlifecycle.Repository {
		if d.cmdRepo == nil {
			d.cmdRepo = newCmdRepository(false) // safe default
		}
		return d.cmdRepo
	}),
)
