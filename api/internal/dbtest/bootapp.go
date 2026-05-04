// api/internal/dbtest/bootapp.go
package dbtest

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/db"
	"local/art-web/api/internal/httpapi"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
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

	// Providers, when non-nil, is the auth.Provider map handed to httpapi.New.
	// Tests inject a deterministic fake here so /auth/{provider}/start emits a
	// real 302 redirect (default empty map → unknown_provider 404, which would
	// lock the wrong contract bytes — see Finding 4 of the round-1 review).
	Providers map[string]auth.Provider
}

// BootApp returns an *httptest.Server backed by the real httpapi.Deps stack
// against a fresh testcontainers Postgres + a local-filesystem store under
// t.TempDir(). It mirrors httpapi/testutil_test.go's setup but is exported so
// the contract suite can use it.
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
	pool, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	// NOTE: StartPostgres shares one container via sync.Once. If multiple tests
	// in the same binary both call BootApp concurrently, the second TruncateAll
	// will wipe data seeded by the first. Call BootApp once per suite invocation
	// (not once per test case), or migrate to per-call sub-schemas if isolation
	// is required. PR 0.7's contract suite only seeds once via /dev/seed.
	TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})

	store := storage.NewLocalFS(t.TempDir())
	clock := time.Now
	if !opts.FixedNow.IsZero() {
		clock = func() time.Time { return opts.FixedNow }
	}
	jwts := auth.NewJWT(opts.JWTKey, clock)
	urls := auth.NewURLBuilder("http://localhost:8787", opts.SignKey, clock)
	arts := artwork.NewRepo(pool)
	images := image.NewRepo(pool)
	imgSvc := image.NewService(store, images, arts)
	upload := image.NewHandler(imgSvc, arts, urls)
	vis := artwork.NewVisibilityService(arts, store)

	providers := opts.Providers
	if providers == nil {
		providers = map[string]auth.Provider{}
	}
	router := httpapi.New(&httpapi.Deps{
		AppEnv:        opts.AppEnv,
		JWT:           jwts,
		URL:           urls,
		Providers:     providers,
		Users:         user.NewRepo(pool),
		Artworks:      arts,
		Tags:          artwork.NewTagsRepo(pool),
		Images:        images,
		Store:         store,
		Upload:        upload,
		Vis:           vis,
		Frontend:      opts.Frontend,
		AllowedOrigin: opts.AllowedOrigin,
		CookieOpts:    auth.CookieOpts{Secure: false},
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}
