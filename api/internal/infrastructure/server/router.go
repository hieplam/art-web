package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	artworkservice "local/art-web/api/internal/artwork/service"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authports "local/art-web/api/internal/auth/ports"
	authservice "local/art-web/api/internal/auth/service"
	imagehttp "local/art-web/api/internal/image/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

type Deps struct {
	AppEnv        string
	JWT           *authservice.JWT
	URL           *signing.URLBuilder
	Providers     map[string]authports.Provider
	Users         *userpostgres.Repo
	Artworks      *artworkpostgres.Repo
	Tags          *artworkpostgres.TagsRepo
	Images        *imagepostgres.Repo
	Store         infrastorage.Storage
	Upload        *imagehttp.Handler
	Vis           *artworkservice.VisibilityService
	Frontend      string
	AllowedOrigin string
	CookieOpts    authhttp.CookieOpts
}

func New(d *Deps) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(CORSFor(d.AllowedOrigin))
	r.Use(authhttp.Middleware(d.JWT))
	r.Use(CacheControlByPath())

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/auth", func(r chi.Router) {
		r.Get("/{provider}/start", authhttp.StartHandler(d.Providers, d.CookieOpts).ServeHTTP)
		r.Get("/{provider}/callback", authhttp.CallbackHandler(d.Providers, d.Users, d.JWT, d.Frontend, d.CookieOpts).ServeHTTP)
		r.Post("/logout", authhttp.LogoutHandler(d.CookieOpts).ServeHTTP)
	})
	r.Get("/me", meHandler(d).ServeHTTP)

	r.Route("/artworks", func(r chi.Router) {
		r.Get("/", listFeedHandler(d).ServeHTTP)
		r.With(authhttp.RequireUser).Post("/", createArtworkHandler(d).ServeHTTP)
		r.Get("/{id}", getArtworkHandler(d).ServeHTTP)
		r.With(authhttp.RequireUser).Patch("/{id}", patchArtworkHandler(d).ServeHTTP)
		r.With(authhttp.RequireUser).Delete("/{id}", deleteArtworkHandler(d).ServeHTTP)
		r.With(authhttp.RequireUser).Post("/{id}/images", d.Upload.Upload)
	})
	r.Get("/users/{slug}", userProfileHandler(d).ServeHTTP)
	r.Get("/tags/{name}", tagsHandler(d).ServeHTTP)

	if d.AppEnv == "test" {
		seed := &DevSeed{
			AppEnv: d.AppEnv, Users: d.Users, Artworks: d.Artworks,
			Tags: d.Tags, Images: d.Images, Store: d.Store,
			JWT: d.JWT, Cookie: d.CookieOpts,
		}
		r.Post("/dev/seed", seed.handle)
	}
	return r
}
