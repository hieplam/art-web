// api/internal/infrastructure/server/registrar.go
package server

import "github.com/go-chi/chi/v5"

// RouteRegistrar lets each slice's *Router type contribute its routes to the
// composed chi router. Implemented by authhttp.Router, userhttp.Router,
// artworkhttp.Router, imagehttp.Router.
type RouteRegistrar interface {
	RegisterRoutes(r chi.Router)
}

// ProvideRouteRegistrars is referenced in cmd/api/wire.go to assemble the
// registrar slice for ComposeRouter. Adding a slice means adding one parameter
// + one slice element here, plus its ProviderSet to wire.Build.
//
// NOTE: this provider returns []RouteRegistrar even though it builds it inline.
// Wire requires the function to exist so the slice can be a Wire-bound value.
func ProvideRouteRegistrars(
// auth, user, artwork, image *Router types fill in here in Task 7+
// when their per-slice routers are introduced.
) []RouteRegistrar {
	return nil
}
