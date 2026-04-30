package cmdlifecycle

import "go.uber.org/fx"

// Module provides cmdlifecycle.Service to fx-wired daemons.
//
// Repository must be supplied separately (the daemon implements it).
var Module = fx.Options(
	fx.Provide(NewService),
)
