package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
)

type Deps struct {
	AppEnv        string
	JWT           *auth.JWT
	URL           *auth.URLBuilder
	Providers     map[string]auth.Provider
	Users         *user.Repo
	Artworks      *artwork.Repo
	Tags          *artwork.TagsRepo
	Images        *image.Repo
	Store         storage.Storage
	Upload        *image.Handler
	Vis           *artwork.VisibilityService
	Frontend      string
	AllowedOrigin string
	CookieOpts    auth.CookieOpts
}

func New(d *Deps) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(CORSFor(d.AllowedOrigin))
	r.Use(auth.Middleware(d.JWT))
	r.Use(CacheControlByPath())

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/auth", func(r chi.Router) {
		r.Get("/{provider}/start", auth.StartHandler(d.Providers, d.CookieOpts).ServeHTTP)
		r.Get("/{provider}/callback", auth.CallbackHandler(d.Providers, d.Users, d.JWT, d.Frontend, d.CookieOpts).ServeHTTP)
		r.Post("/logout", auth.LogoutHandler(d.CookieOpts).ServeHTTP)
	})
	r.Get("/me", meHandler(d).ServeHTTP)

	r.Route("/artworks", func(r chi.Router) {
		r.Get("/", listFeedHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Post("/", createArtworkHandler(d).ServeHTTP)
		r.Get("/{id}", getArtworkHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Patch("/{id}", patchArtworkHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Delete("/{id}", deleteArtworkHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Post("/{id}/images", d.Upload.Upload)
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
