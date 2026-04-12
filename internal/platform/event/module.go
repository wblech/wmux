package event

import "go.uber.org/fx"

// Module provides the event bus to the fx dependency injection container.
var Module = fx.Options(fx.Provide(NewBus))
