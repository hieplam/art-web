// api/internal/infrastructure/server/registrar.go
package server

import (
	"github.com/go-chi/chi/v5"

	artworkhttp "local/art-web/api/internal/artwork/adapters/http"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	imagehttp "local/art-web/api/internal/image/adapters/http"
	userhttp "local/art-web/api/internal/user/adapters/http"
)

// RouteRegistrar lets each slice's *Router type contribute its routes to the
// composed chi router. Implemented by authhttp.Router, userhttp.Router,
// artworkhttp.Router, imagehttp.Router.
type RouteRegistrar interface {
	RegisterRoutes(r chi.Router)
}

// ProvideRouteRegistrars assembles the slice routers into the slice consumed
// by ComposeRouter. Adding a slice means adding one parameter and one element
// here, plus its ProviderSet to wire.Build.
func ProvideRouteRegistrars(
	auth *authhttp.Router,
	user *userhttp.Router,
	artwork *artworkhttp.Router,
	image *imagehttp.Router,
) []RouteRegistrar {
	return []RouteRegistrar{auth, user, artwork, image}
}
