// api/internal/user/adapters/http/handler_integration_test.go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/rs/zerolog"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authservice "local/art-web/api/internal/auth/service"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	"local/art-web/api/internal/infrastructure/database"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	infratest "local/art-web/api/internal/infrastructure/testing"
	userhttp "local/art-web/api/internal/user/adapters/http"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

type intEnv struct {
	router   chi.Router
	jwts     *authservice.JWT
	aliceTok string
	bobTok   string
	slug     string // alice's slug
}

func newIntEnv(t *testing.T) *intEnv {
	t.Helper()
	db, cleanup, err := database.NewGormDBFromDSN(infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	t.Cleanup(cleanup)
	infratest.TruncateAllGorm(t, db)

	users := userpostgres.NewRepo(db)
	arts := artworkpostgres.NewRepo(db)
	images := imagepostgres.NewRepo(db)

	suffix := uuid.NewString()[:4]
	aliceSlug := "alice-" + suffix
	aliceID, err := users.UpsertOAuth(t.Context(), "test", "alice-"+suffix, "alice@test", aliceSlug, "")
	if err != nil {
		t.Fatalf("upsert alice: %v", err)
	}
	bobID, err := users.UpsertOAuth(t.Context(), "test", "bob-"+suffix, "bob@test", "bob-"+suffix, "")
	if err != nil {
		t.Fatalf("upsert bob: %v", err)
	}

	// Seed an artwork + image for alice so the profile handler's cover-loading
	// branch is reachable.
	pubID, err := arts.Create(t.Context(), aliceID, "P", "", "public")
	if err != nil {
		t.Fatalf("create P: %v", err)
	}
	store := infrastorage.NewLocalFS(t.TempDir())
	imgID := uuid.NewString()
	key := "public/" + pubID + "/" + imgID + ".png"
	if err := store.Put(t.Context(), key, strings.NewReader("bytes"), "image/png"); err != nil {
		t.Fatalf("seed image put: %v", err)
	}
	if _, err := images.Insert(t.Context(), imagepostgres.InsertInput{
		ID: imgID, ArtworkID: pubID, ClientImageID: "K",
		ContentType: "image/png", StorageKey: key, SourceSHA256: "sha",
		Position: 0, Width: 1, Height: 1, ByteSize: 5, Blurhash: "L00",
	}); err != nil {
		t.Fatalf("seed image insert: %v", err)
	}

	jwts := authservice.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil)
	urls := signing.NewURLBuilder("http://x", []byte("k"), nil)

	mw := authhttp.NewMiddleware(jwts)
	h := userhttp.NewHandler(users, arts, images, urls, zerolog.Nop())
	router := userhttp.NewRouter(h)

	r := chi.NewRouter()
	r.Use(mw.ParseToken())
	router.RegisterRoutes(r)

	aliceTok, _ := jwts.Issue(aliceID, time.Hour)
	bobTok, _ := jwts.Issue(bobID, time.Hour)
	return &intEnv{router: r, jwts: jwts, aliceTok: aliceTok, bobTok: bobTok, slug: aliceSlug}
}

func (e *intEnv) do(t *testing.T, path, tok string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if tok != "" {
		req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func TestIntegration_Profile_Owner(t *testing.T) {
	e := newIntEnv(t)
	rec := e.do(t, "/users/"+e.slug, e.aliceTok)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Profile_Anon(t *testing.T) {
	e := newIntEnv(t)
	rec := e.do(t, "/users/"+e.slug, "")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Profile_NotFound(t *testing.T) {
	e := newIntEnv(t)
	rec := e.do(t, "/users/no-such-slug", "")
	if rec.Code != 404 {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestIntegration_Profile_BadCursor(t *testing.T) {
	e := newIntEnv(t)
	rec := e.do(t, "/users/"+e.slug+"?cursor=not-base64!!", "")
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}
