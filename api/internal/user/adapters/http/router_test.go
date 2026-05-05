// api/internal/user/adapters/http/router_test.go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	userhttp "local/art-web/api/internal/user/adapters/http"
)

// TestRouter_RegistersUsersSlugRoute confirms the Router mounts
// /users/{slug} on the supplied chi router. With nil collaborators the
// handler will fail at the GetBySlug call — but the route MUST be reachable
// for that failure to surface, so reaching the handler at all (even via a
// later panic) is sufficient evidence the route is registered.
func TestRouter_RegistersUsersSlugRoute(t *testing.T) {
	// We construct the Router with nil collaborators because we only need to
	// verify the route mounts. Invoking the route would panic — instead we
	// check that the chi router routes the request to the handler at all by
	// using a stand-in handler that records its invocation.
	//
	// chi exposes router.Routes() so we can inspect the route table without
	// invoking handlers.
	h := userhttp.NewHandler(nil, nil, nil, nil)
	router := userhttp.NewRouter(h)

	r := chi.NewRouter()
	router.RegisterRoutes(r)

	if !routePresent(r, http.MethodGet, "/users/{slug}") {
		t.Fatalf("/users/{slug} not mounted in chi tree: %+v", r.Routes())
	}
}

// routePresent walks the chi.Routes() tree and returns true if (method, pattern)
// appears.
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

// silence unused-import warning if any
var _ = httptest.NewRecorder
