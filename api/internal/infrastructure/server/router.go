// api/internal/infrastructure/server/router.go
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	authhttp "local/art-web/api/internal/auth/adapters/http"
	"local/art-web/api/internal/auth/service"
)

// AppEnv is a typed string so Wire can distinguish it from other stringly-typed
// providers. Routers gate test-only behavior on this; production wires "prod".
type AppEnv string

// AllowedOrigin is a typed string for the same reason as AppEnv.
type AllowedOrigin string

// Deps is the legacy bundle accepted by New. New code uses NewRouter, which
// takes the slice routers via Wire. Deps stays for existing tests
// (testutil_test.go, bootapp.go) that haven't migrated yet.
type Deps struct {
	AppEnv        string
	JWT           *service.JWT
	Logger        zerolog.Logger
	Frontend      string
	AllowedOrigin string
	CookieOpts    authhttp.CookieOpts

	// Registrars carry the per-slice route registrations. Tests that build
	// their own registrar slice pass it here; production wiring goes through
	// NewRouter directly.
	Registrars []RouteRegistrar
}

// NewRouter assembles a chi router from the cross-cutting middleware stack +
// the slice route registrars. Wire calls this once at app boot. The caller
// (cmd/api or BootApp) wraps the result into *http.Server.
//
// devseed is a test-only shim: when non-nil AND appEnv == "test", a thin
// /dev/seed route is mounted that delegates to seeder.Run. Production passes
// nil and the route stays unregistered. See devseed.go for rationale.
func NewRouter(
	mw *authhttp.Middleware,
	registrars []RouteRegistrar,
	allowed AllowedOrigin,
	appEnv AppEnv,
	devseed *DevSeed,
) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(CORSFor(string(allowed)))
	r.Use(mw.ParseToken())
	r.Use(CacheControlByPath())

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	for _, rr := range registrars {
		rr.RegisterRoutes(r)
	}

	if appEnv == "test" && devseed != nil {
		r.Post("/dev/seed", devseed.Handle)
	}
	return r
}

// New is the legacy entry point retained for tests that haven't migrated to
// NewRouter. It builds the same router by routing through NewRouter with a
// hand-assembled middleware + registrars.
func New(d *Deps) chi.Router {
	mw := authhttp.NewMiddleware(d.JWT)
	return NewRouter(
		mw,
		d.Registrars,
		AllowedOrigin(d.AllowedOrigin),
		AppEnv(d.AppEnv),
		nil,
	)
}
