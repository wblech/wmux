package config

import "go.uber.org/fx"

// Module provides the Config via fx dependency injection.
// It expects the config file path to be provided as a named string dependency.
var Module = fx.Options(
	fx.Provide(Load),
)
