// api/internal/artwork/adapters/http/handler_integration_test.go
//
// Integration tests covering each artwork handler method against a real
// testcontainers Postgres + local-fs storage. The set of cases mirrors the
// privacy-matrix coverage in infrastructure/server but lives here so the
// per-slice coverage gate (`make test-cover-slices`) can attribute the
// handler statements to the artwork slice.
package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	artworkhttp "local/art-web/api/internal/artwork/adapters/http"
	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	artworkservice "local/art-web/api/internal/artwork/service"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authservice "local/art-web/api/internal/auth/service"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	"local/art-web/api/internal/infrastructure/database"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	infratest "local/art-web/api/internal/infrastructure/testing"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

type intEnv struct {
	router    chi.Router
	jwts      *authservice.JWT
	users     *userpostgres.Repo
	arts      *artworkpostgres.Repo
	tags      *artworkpostgres.TagsRepo
	images    *imagepostgres.Repo
	store     infrastorage.Storage
	urls      *signing.URLBuilder
	aliceID   string
	bobID     string
	aliceTok  string
	pubID     string
	privID    string
	tagged    string // tag name applied to pubID
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
	tags := artworkpostgres.NewTagsRepo(db)
	images := imagepostgres.NewRepo(db)

	suffix := uuid.NewString()[:4]
	aliceID, err := users.UpsertOAuth(t.Context(), "test", "alice-"+suffix, "alice@test", "alice-"+suffix, "")
	if err != nil {
		t.Fatalf("upsert alice: %v", err)
	}
	bobID, err := users.UpsertOAuth(t.Context(), "test", "bob-"+suffix, "bob@test", "bob-"+suffix, "")
	if err != nil {
		t.Fatalf("upsert bob: %v", err)
	}

	pubID, err := arts.Create(t.Context(), aliceID, "P", "", "public")
	if err != nil {
		t.Fatalf("create P: %v", err)
	}
	privID, err := arts.Create(t.Context(), aliceID, "Q", "", "private")
	if err != nil {
		t.Fatalf("create Q: %v", err)
	}

	if err := tags.SetTags(t.Context(), pubID, []string{"t"}); err != nil {
		t.Fatalf("tag P: %v", err)
	}

	store := infrastorage.NewLocalFS(t.TempDir())
	jwts := authservice.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil)
	urls := signing.NewURLBuilder("http://x", []byte("k"), nil)
	vis := artworkservice.NewVisibilityService(arts, store, nil)

	// Attach a seed image to the public artwork so the feed/detail handlers'
	// cover-rendering branches get exercised.
	imageID := uuid.NewString()
	key := "public/" + pubID + "/" + imageID + ".png"
	if err := store.Put(t.Context(), key, strings.NewReader("bytes"), "image/png"); err != nil {
		t.Fatalf("seed image put: %v", err)
	}
	if _, err := images.Insert(t.Context(), imagepostgres.InsertInput{
		ID: imageID, ArtworkID: pubID, ClientImageID: "K", ContentType: "image/png",
		StorageKey: key, SourceSHA256: "sha", Position: 0,
		Width: 1, Height: 1, ByteSize: 5, Blurhash: "L00",
	}); err != nil {
		t.Fatalf("seed image insert: %v", err)
	}

	mw := authhttp.NewMiddleware(jwts)
	h := artworkhttp.NewHandler(arts, tags, users, images, vis, urls)
	router := artworkhttp.NewRouter(h, mw)

	r := chi.NewRouter()
	r.Use(mw.ParseToken())
	router.RegisterRoutes(r)

	aliceTok, err := jwts.Issue(aliceID, time.Hour)
	if err != nil {
		t.Fatalf("issue alice: %v", err)
	}

	return &intEnv{
		router: r, jwts: jwts, users: users, arts: arts, tags: tags, images: images,
		store: store, urls: urls,
		aliceID: aliceID, bobID: bobID, aliceTok: aliceTok,
		pubID: pubID, privID: privID, tagged: "t",
	}
}

func (e *intEnv) doJSON(t *testing.T, method, path, body, tok string) (*httptest.ResponseRecorder, *http.Response) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec, rec.Result()
}

func TestIntegration_ListFeed(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/artworks", "", "")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), e.pubID) {
		t.Errorf("public artwork %s missing from feed; body=%s", e.pubID, rec.Body.String())
	}
}

func TestIntegration_ListFeed_BadCursor(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/artworks?cursor=not-base64!!", "", "")
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestIntegration_GetArtwork_Public_Anon(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/artworks/"+e.pubID, "", "")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_GetArtwork_Private_NonOwner(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/artworks/"+e.privID, "", "")
	if rec.Code != 404 {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestIntegration_GetArtwork_Private_Owner(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/artworks/"+e.privID, "", e.aliceTok)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Create_BadJSON(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "POST", "/artworks", "not json", e.aliceTok)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestIntegration_Create_BadVisibility(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "POST", "/artworks", `{"title":"x","visibility":"draft"}`, e.aliceTok)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestIntegration_Create_HappyPath(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "POST", "/artworks", `{"title":"new","visibility":"public","tags":["a"]}`, e.aliceTok)
	if rec.Code != 201 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Patch_NonOwner_404(t *testing.T) {
	e := newIntEnv(t)
	bobTok, _ := e.jwts.Issue(e.bobID, time.Hour)
	rec, _ := e.doJSON(t, "PATCH", "/artworks/"+e.pubID, `{"title":"x"}`, bobTok)
	if rec.Code != 404 {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestIntegration_Patch_BadJSON(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "PATCH", "/artworks/"+e.pubID, "not json", e.aliceTok)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestIntegration_Patch_TitleAndDescriptionAndCoverAndTags(t *testing.T) {
	e := newIntEnv(t)
	body := `{"title":"r","description":"d","cover_position":0,"tags":["a","b"]}`
	rec, _ := e.doJSON(t, "PATCH", "/artworks/"+e.privID, body, e.aliceTok)
	if rec.Code != 204 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIntegration_Patch_BadCoverPosition(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "PATCH", "/artworks/"+e.privID, `{"cover_position":-1}`, e.aliceTok)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestIntegration_Patch_BadVisibility(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "PATCH", "/artworks/"+e.privID, `{"visibility":"draft"}`, e.aliceTok)
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestIntegration_Delete_NonOwner_404(t *testing.T) {
	e := newIntEnv(t)
	bobTok, _ := e.jwts.Issue(e.bobID, time.Hour)
	rec, _ := e.doJSON(t, "DELETE", "/artworks/"+e.pubID, "", bobTok)
	if rec.Code != 404 {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestIntegration_Delete_Owner(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "DELETE", "/artworks/"+e.privID, "", e.aliceTok)
	if rec.Code != 204 {
		t.Fatalf("status=%d want 204", rec.Code)
	}
}

func TestIntegration_Tag(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/tags/"+e.tagged, "", "")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), e.pubID) {
		t.Errorf("tagged feed should contain pubID; body=%s", rec.Body.String())
	}
}

func TestIntegration_Tag_BadCursor(t *testing.T) {
	e := newIntEnv(t)
	rec, _ := e.doJSON(t, "GET", "/tags/t?cursor=not-base64!!", "", "")
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}
