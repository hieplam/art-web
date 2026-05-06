// api/internal/infrastructure/testing/bootapp.go
package testing

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	artworkhttp "local/art-web/api/internal/artwork/adapters/http"
	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	artworkservice "local/art-web/api/internal/artwork/service"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authports "local/art-web/api/internal/auth/ports"
	authservice "local/art-web/api/internal/auth/service"
	imagehttp "local/art-web/api/internal/image/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	imageservice "local/art-web/api/internal/image/service"
	"local/art-web/api/internal/infrastructure/database"
	"local/art-web/api/internal/infrastructure/server"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	userhttp "local/art-web/api/internal/user/adapters/http"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

// BootOpts injects deterministic sources for the contract suite. Any zero
// field falls back to the current production default.
type BootOpts struct {
	// FixedNow, when non-zero, replaces every JWT/URL clock with a constant.
	FixedNow time.Time

	// AppEnv defaults to "test" so the dev seed route is registered.
	AppEnv string

	// Frontend defaults to "http://localhost:3000/".
	Frontend string

	// AllowedOrigin defaults to "http://localhost:3000".
	AllowedOrigin string

	// JWTKey / SignKey default to fixed 32-byte test keys.
	JWTKey  []byte
	SignKey []byte

	// Providers, when non-nil, is the authports.OAuthProvider map handed to
	// the auth handler. Tests inject a deterministic fake here so
	// /auth/{provider}/start emits a real 302 redirect (default empty map →
	// unknown_provider 404, which would lock the wrong contract bytes — see
	// Finding 4 of the round-1 review).
	Providers map[string]authports.OAuthProvider

	// IDProvider injects a deterministic image-ID source. nil → production default.
	IDProvider imageservice.IDProvider

	// RandReader injects a deterministic random source for OAuth state cookies.
	// nil → crypto/rand.Reader (production default).
	RandReader authhttp.RandReader
}

// BootApp returns an *httptest.Server backed by the slice routers + server
// composition against a fresh testcontainers Postgres + a local-filesystem
// store under t.TempDir(). It mirrors the production wire setup but builds
// each piece explicitly so the contract suite can substitute deterministic
// providers without going through wire_gen.go.
func BootApp(t testing.TB, opts BootOpts) *httptest.Server {
	t.Helper()

	if opts.AppEnv == "" {
		opts.AppEnv = "test"
	}
	if opts.Frontend == "" {
		opts.Frontend = "http://localhost:3000/"
	}
	if opts.AllowedOrigin == "" {
		opts.AllowedOrigin = "http://localhost:3000"
	}
	if len(opts.JWTKey) == 0 {
		opts.JWTKey = []byte("test-jwt-key-pad-to-32-bytes!!!!")
	}
	if len(opts.SignKey) == 0 {
		opts.SignKey = []byte("test-sign-key-pad-to-32-bytes!!!")
	}

	dsn := StartPostgres(t)
	db, cleanup, err := database.NewGormDBFromDSN(dsn)
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	t.Cleanup(cleanup)
	// NOTE: StartPostgres shares one container via sync.Once. If multiple tests
	// in the same binary both call BootApp concurrently, the second TruncateAll
	// will wipe data seeded by the first. Call BootApp once per suite invocation
	// (not once per test case), or migrate to per-call sub-schemas if isolation
	// is required. PR 0.7's contract suite only seeds once via /dev/seed.
	TruncateAllGorm(t, db)

	store := infrastorage.NewLocalFS(t.TempDir())
	clock := time.Now
	if !opts.FixedNow.IsZero() {
		clock = func() time.Time { return opts.FixedNow }
	}
	jwts := authservice.NewJWT(opts.JWTKey, clock)
	urls := signing.NewURLBuilder("http://localhost:8787", opts.SignKey, clock)
	users := userpostgres.NewRepo(db)
	arts := artworkpostgres.NewRepo(db)
	tags := artworkpostgres.NewTagsRepo(db)
	images := imagepostgres.NewRepo(db)
	if opts.RandReader != nil {
		t.Cleanup(authhttp.SetStateRandForTest(opts.RandReader))
	}
	ids := opts.IDProvider
	if ids == nil {
		ids = imageservice.NewUUIDProvider()
	}
	imgSvc := imageservice.NewServiceWithIDs(store, images, arts, ids)
	upload := imagehttp.NewHandler(imgSvc, arts, urls, zerolog.Nop())
	vis := artworkservice.NewVisibilityService(arts, store, nil)
	v := server.NewValidator()

	providers := opts.Providers
	if providers == nil {
		providers = map[string]authports.OAuthProvider{}
	}

	authMW := authhttp.NewMiddleware(jwts)
	authH := authhttp.NewHandler(authhttp.AuthDeps{
		Providers:    providers,
		Users:        users,
		JWT:          jwts,
		FrontendHome: authhttp.FrontendHome(opts.Frontend),
		CookieOpts:   authhttp.CookieOpts{Secure: false},
	})
	authR := authhttp.NewRouter(authH)

	userH := userhttp.NewHandler(users, arts, images, urls, zerolog.Nop())
	userR := userhttp.NewRouter(userH)

	artworkH := artworkhttp.NewHandler(arts, tags, users, images, vis, urls, zerolog.Nop(), v)
	artworkR := artworkhttp.NewRouter(artworkH, authMW)

	imageR := imagehttp.NewRouter(upload, authMW)

	registrars := server.ProvideRouteRegistrars(authR, userR, artworkR, imageR)

	devseed := &server.DevSeed{
		AppEnv: opts.AppEnv, Users: users, Artworks: arts,
		Tags: tags, Images: images, Store: store,
		JWT: jwts, Cookie: authhttp.CookieOpts{Secure: false},
	}

	router := server.NewRouter(authMW, registrars, server.AllowedOrigin(opts.AllowedOrigin),
		server.AppEnv(opts.AppEnv), devseed)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}
