package transport

import "go.uber.org/fx"

// Module provides the transport server to the fx dependency injection container.
var Module = fx.Options(fx.Provide(NewServer))
