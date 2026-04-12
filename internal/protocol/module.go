package protocol

import "go.uber.org/fx"

// Module provides the protocol Codec via dependency injection.
var Module = fx.Options(
	fx.Provide(func() Codec { return Codec{} }),
)
