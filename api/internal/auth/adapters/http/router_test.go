// api/internal/auth/adapters/http/router_test.go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	authhttp "local/art-web/api/internal/auth/adapters/http"
	authports "local/art-web/api/internal/auth/ports"
	authservice "local/art-web/api/internal/auth/service"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

// stubUserUpserterRepo is a *userpostgres.Repo-shaped stub for tests that don't
// need a real DB. It implements just the Get method that /me uses; the upsert
// path lives on the Handler indirectly via CallbackHandler and is exercised by
// the integration suite, not here.
//
// The auth Router tests focus on route mounting + dispatch — does the Handler
// type plumb the right subhandler into the right method? The handler bodies
// themselves are already well-covered by handlers_test.go.

// TestRouter_Mounts_RegistersExpectedPaths constructs a Router, mounts it on a
// fresh chi.Router, and probes each path to confirm the dispatch is wired. We
// expect the auth start path to redirect even with an empty providers map
// returns 404 unknown_provider — we just need the route to exist.
func TestRouter_Mounts_RegistersExpectedPaths(t *testing.T) {
	jwts := authservice.NewJWT(testKey, nil)
	h := authhttp.NewHandler(authhttp.AuthDeps{
		Providers:    map[string]authports.OAuthProvider{},
		Users:        (*userpostgres.Repo)(nil),
		JWT:          jwts,
		FrontendHome: authhttp.FrontendHome("http://frontend.test/"),
		CookieOpts:   authhttp.CookieOpts{Secure: false},
	})
	router := authhttp.NewRouter(h)
	r := chi.NewRouter()
	router.RegisterRoutes(r)

	tests := []struct {
		method, path string
		wantStatus   int
	}{
		// /auth/{provider}/start with empty providers map → 404 unknown_provider.
		{"GET", "/auth/google/start", http.StatusNotFound},
		// /auth/{provider}/callback with empty map → 404 unknown_provider.
		{"GET", "/auth/google/callback", http.StatusNotFound},
		// /auth/logout → 204 no content (no auth cookie required).
		{"POST", "/auth/logout", http.StatusNoContent},
		// /me with no cookie → 401 unauthorized.
		{"GET", "/me", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestMiddleware_Methods_Compose covers the *Middleware struct's ParseToken +
// RequireUser methods (currently 0% coverage because the integration suite
// wires them but the slice tests use the legacy package-level functions).
func TestMiddleware_Methods_Compose(t *testing.T) {
	jwts := authservice.NewJWT(testKey, nil)
	mw := authhttp.NewMiddleware(jwts)

	// ParseToken: a request without an auth cookie still calls next, with no
	// user id in the context.
	called := false
	parseHandler := mw.ParseToken()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, ok := authhttp.UserIDFrom(r.Context()); ok {
			t.Fatal("expected no user")
		}
		called = true
	}))
	parseHandler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("ParseToken should call next handler")
	}

	// RequireUser: a request without a user id is 401'd; one with a user id
	// in the context is allowed through.
	gate := mw.RequireUser()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	gate.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("RequireUser without ctx user: status=%d want 401", rec.Code)
	}

	// Build a context with a user-id and verify the gate lets the request
	// through. We can't easily set the ctx key from outside the package, but
	// we can use the ParseToken middleware — issue a JWT and present it as
	// a cookie.
	tok, _ := jwts.Issue("u-1", time.Minute)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	stack := mw.ParseToken()(mw.RequireUser()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, _ := authhttp.UserIDFrom(r.Context()); uid != "u-1" {
			t.Fatalf("uid=%q want u-1", uid)
		}
		w.WriteHeader(http.StatusOK)
	})))
	rec = httptest.NewRecorder()
	stack.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed request: status=%d want 200", rec.Code)
	}
}
