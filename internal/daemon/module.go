package daemon

import "go.uber.org/fx"

// Module provides the daemon to the fx dependency injection container.
var Module = fx.Options(fx.Provide(NewDaemon))
