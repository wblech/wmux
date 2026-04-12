package pty

import "go.uber.org/fx"

// Module provides the Spawner via fx dependency injection.
var Module = fx.Options(
	fx.Provide(func() Spawner {
		return &UnixSpawner{}
	}),
)
