// api/internal/artwork/adapters/http/router_test.go
package http_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	artworkhttp "local/art-web/api/internal/artwork/adapters/http"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authservice "local/art-web/api/internal/auth/service"
)

// TestRouter_MountsAllArtworkRoutes confirms the artwork Router registers each
// route the slice owns at the chi-router level. We don't invoke the handlers
// (they need real collaborators) — we just walk the route table.
func TestRouter_MountsAllArtworkRoutes(t *testing.T) {
	h := artworkhttp.NewHandler(nil, nil, nil, nil, nil, nil)
	mw := authhttp.NewMiddleware(authservice.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil))
	router := artworkhttp.NewRouter(h, mw)

	r := chi.NewRouter()
	router.RegisterRoutes(r)

	want := []struct{ method, pattern string }{
		{http.MethodGet, "/artworks"},
		{http.MethodGet, "/artworks/{id}"},
		{http.MethodPost, "/artworks"},
		{http.MethodPatch, "/artworks/{id}"},
		{http.MethodDelete, "/artworks/{id}"},
		{http.MethodGet, "/tags/{name}"},
	}
	for _, w := range want {
		if !routePresent(r, w.method, w.pattern) {
			t.Errorf("artwork route not mounted: %s %s — chi tree: %+v",
				w.method, w.pattern, r.Routes())
		}
	}
}

func routePresent(r chi.Router, method, pattern string) bool {
	for _, route := range r.Routes() {
		if route.Pattern == pattern {
			if _, ok := route.Handlers[method]; ok {
				return true
			}
		}
	}
	return false
}
