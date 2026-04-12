package logger

import "go.uber.org/fx"

// Module provides a *slog.Logger via fx dependency injection.
var Module = fx.Options(
	fx.Provide(New),
)
