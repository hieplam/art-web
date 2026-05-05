package httpapi_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/httpapi"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
)

// testSeedPNG is a 1×1 transparent PNG (67 bytes).
var testSeedPNG = func() []byte {
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=",
	)
	if err != nil {
		panic(err)
	}
	return b
}()

// testJWTKey is a fixed 32-byte key used in tests.
var testJWTKey = []byte("test-jwt-key-pad-to-32-bytes!!!!")

// testSignKey is a fixed 32-byte key used for URL signing in tests.
var testSignKey = []byte("test-sign-key-pad-to-32-bytes!!!")

// MatrixEnv holds the shared state for privacy matrix integration tests.
type MatrixEnv struct {
	QID         string
	PID         string
	AliceSuffix string
	router      http.Handler
	ownerToken  string
	otherToken  string
}

// setupMatrixEnv creates a full router wired with test deps and seeds two artworks:
// P (public) and Q (private), both owned by Alice. Bob is seeded as another user.
func setupMatrixEnv(t *testing.T) *MatrixEnv {
	t.Helper()
	dsn := dbtest.StartPostgres(t)
	pool, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})

	users := user.NewRepo(pool)
	arts := artwork.NewRepo(pool)
	tags := artwork.NewTagsRepo(pool)
	images := image.NewRepo(pool)

	// Seed Alice (owner) with a unique suffix.
	// Pass "alice-<suffix>" as displayName so slugify produces "alice-<suffix>" as the slug.
	aliceSuffix := randHexN(4)
	aliceID, err := users.UpsertOAuth(t.Context(), "test", "alice-"+aliceSuffix, "alice@test", "alice-"+aliceSuffix, "")
	if err != nil {
		t.Fatalf("upsert alice: %v", err)
	}
	// Seed Bob (other viewer).
	bobID, err := users.UpsertOAuth(t.Context(), "test", "bob-"+aliceSuffix, "bob@test", "bob-"+aliceSuffix, "")
	if err != nil {
		t.Fatalf("upsert bob: %v", err)
	}

	// Create public artwork P and private artwork Q owned by Alice.
	pID, err := arts.Create(t.Context(), aliceID, "Public P", "", "public")
	if err != nil {
		t.Fatalf("create P: %v", err)
	}
	qID, err := arts.Create(t.Context(), aliceID, "Private Q", "", "private")
	if err != nil {
		t.Fatalf("create Q: %v", err)
	}

	// Tag both with "t".
	if err := tags.SetTags(t.Context(), pID, []string{"t"}); err != nil {
		t.Fatalf("tag P: %v", err)
	}
	if err := tags.SetTags(t.Context(), qID, []string{"t"}); err != nil {
		t.Fatalf("tag Q: %v", err)
	}

	// Create local store.
	store := storage.NewLocalFS(t.TempDir())

	// Attach a seed image to P (public) and Q (private) so the feed includes them.
	if err := attachTestImage(t.Context(), store, images, pID, "public"); err != nil {
		t.Fatalf("attach image P: %v", err)
	}
	if err := attachTestImage(t.Context(), store, images, qID, "private"); err != nil {
		t.Fatalf("attach image Q: %v", err)
	}

	jwts := auth.NewJWT(testJWTKey, time.Now)
	urls := auth.NewURLBuilder("http://localhost:8787", testSignKey, time.Now)

	imgSvc := image.NewService(store, images, arts)
	upload := image.NewHandler(imgSvc, arts, urls)
	vis := artwork.NewVisibilityService(arts, store)

	router := httpapi.New(&httpapi.Deps{
		AppEnv:        "test",
		JWT:           jwts,
		URL:           urls,
		Providers:     map[string]auth.Provider{},
		Users:         users,
		Artworks:      arts,
		Tags:          tags,
		Images:        images,
		Store:         store,
		Upload:        upload,
		Vis:           vis,
		Frontend:      "http://localhost:3000/",
		AllowedOrigin: "http://localhost:3000",
		CookieOpts:    auth.CookieOpts{Secure: false},
	})

	aliceTok, err := jwts.Issue(aliceID, time.Hour)
	if err != nil {
		t.Fatalf("issue alice token: %v", err)
	}
	bobTok, err := jwts.Issue(bobID, time.Hour)
	if err != nil {
		t.Fatalf("issue bob token: %v", err)
	}

	return &MatrixEnv{
		QID:         qID,
		PID:         pID,
		AliceSuffix: aliceSuffix,
		router:      router,
		ownerToken:  aliceTok,
		otherToken:  bobTok,
	}
}

// attachTestImage writes a 1×1 seed PNG into the store and inserts the image row.
func attachTestImage(ctx context.Context, store storage.Storage, images *image.Repo, artworkID, visibility string) error {
	imageID := uuid.NewString()
	key := visibility + "/" + artworkID + "/" + imageID + ".png"
	if err := store.Put(ctx, key, bytes.NewReader(testSeedPNG), "image/png"); err != nil {
		return err
	}
	sum := sha256.Sum256(testSeedPNG)
	_, err := images.Insert(ctx, image.InsertInput{
		ID:            imageID,
		ArtworkID:     artworkID,
		ClientImageID: "seed",
		ContentType:   "image/png",
		StorageKey:    key,
		SourceSHA256:  hex.EncodeToString(sum[:]),
		Position:      0,
		Width:         1,
		Height:        1,
		ByteSize:      len(testSeedPNG),
		Blurhash:      "L00000fQfQfQfQfQfQfQfQfQfQfQ",
	})
	return err
}

// request sends a test HTTP request to the router and returns the body and status code.
// viewer must be "owner", "other", or "anon".
func (env *MatrixEnv) request(t *testing.T, viewer, method, path string) (string, int) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	switch viewer {
	case "owner":
		req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	case "other":
		req.AddCookie(&http.Cookie{Name: "auth", Value: env.otherToken})
	case "anon":
		// no cookie
	default:
		t.Fatalf("unknown viewer %q; must be owner, other, or anon", viewer)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	return string(body), rec.Code
}

// requestWithJSONBody sends method+path with the given JSON body string,
// authenticated by viewer. Returns body bytes and status. Used by tests that
// need to send PATCH/POST without short-circuiting on bad_json.
func (env *MatrixEnv) requestWithJSONBody(t *testing.T, viewer, method, path, body string) (string, int) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	switch viewer {
	case "owner":
		req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	case "other":
		req.AddCookie(&http.Cookie{Name: "auth", Value: env.otherToken})
	case "anon":
		// no cookie
	default:
		t.Fatalf("unknown viewer %q", viewer)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return string(b), rec.Code
}

// randHexN returns a hex-encoded string of n random bytes.
func randHexN(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// testDeps creates a full Deps for use in unit tests with the given appEnv.
func testDeps(t *testing.T, appEnv string) *httpapi.Deps {
	t.Helper()
	dsn := dbtest.StartPostgres(t)
	pool, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})

	store := storage.NewLocalFS(t.TempDir())
	jwts := auth.NewJWT(testJWTKey, time.Now)
	urls := auth.NewURLBuilder("http://localhost:8787", testSignKey, time.Now)

	arts := artwork.NewRepo(pool)
	images := image.NewRepo(pool)
	imgSvc := image.NewService(store, images, arts)
	upload := image.NewHandler(imgSvc, arts, urls)
	vis := artwork.NewVisibilityService(arts, store)

	return &httpapi.Deps{
		AppEnv:        appEnv,
		JWT:           jwts,
		URL:           urls,
		Providers:     map[string]auth.Provider{},
		Users:         user.NewRepo(pool),
		Artworks:      arts,
		Tags:          artwork.NewTagsRepo(pool),
		Images:        images,
		Store:         store,
		Upload:        upload,
		Vis:           vis,
		Frontend:      "http://localhost:3000/",
		AllowedOrigin: "http://localhost:3000",
		CookieOpts:    auth.CookieOpts{Secure: false},
	}
}
