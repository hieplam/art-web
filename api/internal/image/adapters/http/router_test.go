// api/internal/image/adapters/http/router_test.go
package http_test

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	authhttp "local/art-web/api/internal/auth/adapters/http"
	authservice "local/art-web/api/internal/auth/service"
	imagehttp "local/art-web/api/internal/image/adapters/http"
)

// TestRouter_MountsUploadRoute confirms the image Router registers
// POST /artworks/{id}/images.
func TestRouter_MountsUploadRoute(t *testing.T) {
	h := imagehttp.NewHandler(nil, nil, nil)
	mw := authhttp.NewMiddleware(authservice.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil))
	router := imagehttp.NewRouter(h, mw)

	r := chi.NewRouter()
	router.RegisterRoutes(r)

	if !routePresent(r, http.MethodPost, "/artworks/{id}/images") {
		t.Fatalf("upload route not mounted: %+v", r.Routes())
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
