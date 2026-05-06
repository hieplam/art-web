// api/internal/infrastructure/server/providers.go
package server

import (
	"github.com/go-playground/validator/v10"
	"github.com/google/wire"
)

// NewValidator creates a new validator instance for use by slice handlers.
func NewValidator() *validator.Validate {
	return validator.New()
}

// ProviderSet exposes the server-composition constructors: ProvideRouteRegistrars
// + NewRouter. Adapter slices supply their *Router types upstream via their own
// ProviderSets, and ProvideRouteRegistrars stitches them into the
// []RouteRegistrar that NewRouter consumes.
var ProviderSet = wire.NewSet(
	NewValidator,
	ProvideRouteRegistrars,
	NewRouter,
)
