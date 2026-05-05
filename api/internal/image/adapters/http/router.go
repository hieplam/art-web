package http

import (
	"github.com/go-chi/chi/v5"

	authhttp "local/art-web/api/internal/auth/adapters/http"
)

// Router exposes the image slice's HTTP routes via the RouteRegistrar
// contract. Currently the slice owns one route — POST /artworks/{id}/images —
// even though its path lives under the /artworks/ namespace, because the
// handler logic is image-shaped (multipart parsing, blurhash, sha256, idempotent
// inserts, decoder).
type Router struct {
	handler *Handler
	auth    *authhttp.Middleware
}

// NewRouter pairs the image upload Handler with the auth middleware so the
// route can be gated by RequireUser.
func NewRouter(h *Handler, auth *authhttp.Middleware) *Router {
	return &Router{handler: h, auth: auth}
}

// RegisterRoutes mounts POST /artworks/{id}/images on the supplied router.
func (router *Router) RegisterRoutes(r chi.Router) {
	r.With(router.auth.RequireUser()).Post("/artworks/{id}/images", router.handler.Upload)
}
