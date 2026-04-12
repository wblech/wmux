package config

import "go.uber.org/fx"

// Path is a named type for the config file path, used for fx injection.
type Path string

// Module provides the Config via fx dependency injection.
// Requires a Path value in the fx container.
var Module = fx.Options(
	fx.Provide(func(p Path) (*Config, error) {
		return Load(string(p))
	}),
)
