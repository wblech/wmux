package session

import "go.uber.org/fx"

// Module provides the session service to the fx dependency injection container.
var Module = fx.Options(fx.Provide(NewService))
