// api/internal/infrastructure/server/providers.go
package server

import "github.com/google/wire"

// ProviderSet exposes the server-composition constructors: ProvideRouteRegistrars
// + NewRouter. Adapter slices supply their *Router types upstream via their own
// ProviderSets, and ProvideRouteRegistrars stitches them into the
// []RouteRegistrar that NewRouter consumes.
var ProviderSet = wire.NewSet(
	ProvideRouteRegistrars,
	NewRouter,
)
