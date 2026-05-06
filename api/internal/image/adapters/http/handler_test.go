package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authservice "local/art-web/api/internal/auth/service"
	imagehttp "local/art-web/api/internal/image/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	imageservice "local/art-web/api/internal/image/service"
	"local/art-web/api/internal/infrastructure/database"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	infratest "local/art-web/api/internal/infrastructure/testing"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

type failingStore struct {
	infrastorage.Storage
	failOn int
	count  int
	mu     sync.Mutex
}

func (f *failingStore) Put(ctx context.Context, k string, b io.Reader, ct string) error {
	f.mu.Lock()
	f.count++
	n := f.count
	f.mu.Unlock()
	if n == f.failOn {
		return fmt.Errorf("simulated failure on call %d", n)
	}
	return f.Storage.Put(ctx, k, b, ct)
}

type spyStore struct {
	infrastorage.Storage
	mu         sync.Mutex
	putCount   int
	deleteKeys []string
}

func (s *spyStore) Put(ctx context.Context, k string, b io.Reader, ct string) error {
	s.mu.Lock()
	s.putCount++
	s.mu.Unlock()
	return s.Storage.Put(ctx, k, b, ct)
}

func (s *spyStore) Delete(ctx context.Context, k string) error {
	s.mu.Lock()
	s.deleteKeys = append(s.deleteKeys, k)
	s.mu.Unlock()
	return s.Storage.Delete(ctx, k)
}

func newHandler(t *testing.T, store infrastorage.Storage) (*chi.Mux, string, string) {
	db, cleanup, err := database.NewGormDBFromDSN(infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	t.Cleanup(cleanup)
	infratest.TruncateAllGorm(t, db)
	users := userpostgres.NewRepo(db)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S", "a@b", "alice", "")
	arts := artworkpostgres.NewRepo(db)
	aid, _ := arts.Create(t.Context(), uid, "x", "", "private")

	images := imagepostgres.NewRepo(db)
	svc := imageservice.NewService(store, images, arts)
	jwts := authservice.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil)
	h := imagehttp.NewHandler(svc, arts, signing.NewURLBuilder("http://x", []byte("k"), nil), zerolog.Nop())

	r := chi.NewRouter()
	r.Use(authhttp.ParseTokenMiddleware(jwts))
	r.With(authhttp.RequireUser).Post("/artworks/{id}/images", h.Upload)
	tok, _ := jwts.Issue(uid, 3600*time.Second)
	return r, aid, tok
}

func uploadBytes(
	t *testing.T,
	mux http.Handler,
	artID, token, clientID string,
	position int,
	declaredType, filename string,
	raw []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	manifest := fmt.Sprintf(
		`[{"client_image_id":"%s","position":%d,"content_type":"%s"}]`,
		clientID, position, declaredType,
	)
	_ = mw.WriteField("manifest", manifest)
	w, _ := mw.CreateFormFile("files", filename)
	_, _ = w.Write(raw)
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+artID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func uploadJPEG(t *testing.T, mux http.Handler, artID, token, clientID string, position int) int {
	t.Helper()
	return uploadJPEGResponse(t, mux, artID, token, clientID, position).Code
}

func uploadJPEGResponse(t *testing.T, mux http.Handler, artID, token, clientID string, position int) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := os.ReadFile("testdata/sample.jpg")
	return uploadBytes(t, mux, artID, token, clientID, position, "image/jpeg", "f.jpg", raw)
}

func requireJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Result().Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type=%q, want application/json", got)
	}
}

func decodeUploadResult(t *testing.T, rec *httptest.ResponseRecorder) []struct {
	ID      string `json:"id"`
	Existed bool   `json:"existed"`
} {
	t.Helper()
	var out []struct {
		ID      string `json:"id"`
		Existed bool   `json:"existed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode upload result: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one upload result, got %d", len(out))
	}
	return out
}

func TestUploadFirstCreate201Retry200(t *testing.T) {
	store := infrastorage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	rec := uploadJPEGResponse(t, mux, aid, token, "K1", 0)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first upload status=%d want 201", rec.Code)
	}
	requireJSONContentType(t, rec)

	retry := uploadJPEGResponse(t, mux, aid, token, "K1", 0)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry upload status=%d want 200", retry.Code)
	}
	requireJSONContentType(t, retry)
}

func TestUploadNonMultipart_Returns415JSON(t *testing.T) {
	store := infrastorage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	req := httptest.NewRequest("POST", "/artworks/"+aid+"/images", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d want 415", rec.Code)
	}
	requireJSONContentType(t, rec)
}

func TestUploadInvalidImage_Returns422JSON(t *testing.T) {
	store := infrastorage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	rec := uploadBytes(t, mux, aid, token, "K1", 0, "image/jpeg", "bad.jpg", []byte("not an image"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d want 422", rec.Code)
	}
	requireJSONContentType(t, rec)
}

func TestUploadFingerprintMismatch_SkipsDecodeAndStorageWrite(t *testing.T) {
	spy := &spyStore{Storage: infrastorage.NewLocalFS(t.TempDir())}
	mux, aid, token := newHandler(t, spy)

	first := uploadJPEGResponse(t, mux, aid, token, "K1", 0)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload: %d", first.Code)
	}
	putAfterFirst := spy.putCount

	rec := uploadBytes(t, mux, aid, token, "K1", 0, "image/jpeg", "bad.jpg", []byte("not an image"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409", rec.Code)
	}
	requireJSONContentType(t, rec)
	if spy.putCount != putAfterFirst {
		t.Fatalf("fingerprint mismatch must not call store.Put: putCount was %d after first, %d after retry",
			putAfterFirst, spy.putCount)
	}
}

func TestUploadStorageKeyUsesReturnedImageIDAndRetryStable(t *testing.T) {
	store := infrastorage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	first := uploadJPEGResponse(t, mux, aid, token, "K1", 0)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload: %d", first.Code)
	}
	firstOut := decodeUploadResult(t, first)
	key := "private/" + aid + "/" + firstOut[0].ID + ".jpg"
	ok, err := store.Exists(t.Context(), key)
	if err != nil {
		t.Fatalf("Exists(%q): %v", key, err)
	}
	if !ok {
		t.Fatalf("storage key %q was not written", key)
	}

	retry := uploadJPEGResponse(t, mux, aid, token, "K1", 0)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry upload: %d", retry.Code)
	}
	retryOut := decodeUploadResult(t, retry)
	if retryOut[0].ID != firstOut[0].ID || !retryOut[0].Existed {
		t.Fatalf("retry returned %+v, want same id %q with existed=true", retryOut[0], firstOut[0].ID)
	}
}

func TestUploadCase19_ConcurrentSamePosition(t *testing.T) {
	store := infrastorage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	const N = 10
	var wg sync.WaitGroup
	codes := make([]int, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = uploadJPEG(t, mux, aid, token, "K"+strconv.Itoa(i), 5)
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case 200, 201:
			ok++
		case 412:
			conflict++
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("expected exactly 1 success, got %d (codes=%v)", ok, codes)
	}
	if conflict != N-1 {
		t.Fatalf("expected %d conflicts, got %d (codes=%v)", N-1, conflict, codes)
	}
}

func TestUploadCase20_PartialFailureResume(t *testing.T) {
	base := infrastorage.NewLocalFS(t.TempDir())
	store := &failingStore{Storage: base, failOn: 3}
	mux, aid, token := newHandler(t, store)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	manifest := []map[string]any{}
	for i := 0; i < 5; i++ {
		manifest = append(manifest, map[string]any{
			"client_image_id": "K" + strconv.Itoa(i),
			"position":        i,
			"content_type":    "image/jpeg",
		})
	}
	mb, _ := json.Marshal(manifest)
	_ = mw.WriteField("manifest", string(mb))
	raw, _ := os.ReadFile("testdata/sample.jpg")
	for i := 0; i < 5; i++ {
		w, _ := mw.CreateFormFile("files", fmt.Sprintf("f%d.jpg", i))
		_, _ = w.Write(raw)
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+aid+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Fatal("first batch should have failed at file index 2")
	}

	store.failOn = 0

	body2 := &bytes.Buffer{}
	mw2 := multipart.NewWriter(body2)
	_ = mw2.WriteField("manifest", string(mb))
	for i := 0; i < 5; i++ {
		w, _ := mw2.CreateFormFile("files", fmt.Sprintf("f%d.jpg", i))
		_, _ = w.Write(raw)
	}
	mw2.Close()
	req2 := httptest.NewRequest("POST", "/artworks/"+aid+"/images", body2)
	req2.Header.Set("Content-Type", mw2.FormDataContentType())
	req2.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != 200 && rec2.Code != 201 {
		t.Fatalf("retry status %d", rec2.Code)
	}

	// infratest.StartPostgres is sync.Once: the second call returns the same DSN as
	// newHandler used, so this DB reads the same database. Tests must not run
	// in parallel with other tests that call TruncateAll on the shared container.
	verifyDB, vClean, err := database.NewGormDBFromDSN(infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	t.Cleanup(vClean)
	var n int64
	if err := verifyDB.WithContext(t.Context()).Table("artwork_images").
		Where("artwork_id = ?", aid).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5 rows, got %d", n)
	}
}

func TestUploadContentTypeMismatch_Returns422(t *testing.T) {
	store := infrastorage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	// Send a real JPEG but declare content_type as image/png — must be rejected.
	raw, _ := os.ReadFile("testdata/sample.jpg")
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("manifest", `[{"client_image_id":"K1","position":0,"content_type":"image/png"}]`)
	w, _ := mw.CreateFormFile("files", "f.jpg")
	_, _ = w.Write(raw)
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+aid+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 422 {
		t.Fatalf("expected 422 for content-type mismatch, got %d", rec.Code)
	}
	requireJSONContentType(t, rec)
}

func TestUploadIdempotentRetry_SkipsStorageWrite(t *testing.T) {
	spy := &spyStore{Storage: infrastorage.NewLocalFS(t.TempDir())}
	mux, aid, token := newHandler(t, spy)

	if code := uploadJPEG(t, mux, aid, token, "K1", 0); code != 200 && code != 201 {
		t.Fatalf("first upload: %d", code)
	}
	putAfterFirst := spy.putCount

	// Retry with identical client_image_id and bytes — pre-check must short-circuit before Put.
	if code := uploadJPEG(t, mux, aid, token, "K1", 0); code != 200 {
		t.Fatalf("retry upload: %d", code)
	}
	if spy.putCount != putAfterFirst {
		t.Fatalf("idempotent retry must not call store.Put: putCount was %d after first, %d after retry",
			putAfterFirst, spy.putCount)
	}
}

func TestUploadPositionConflict_OrphanIsDeleted(t *testing.T) {
	spy := &spyStore{Storage: infrastorage.NewLocalFS(t.TempDir())}
	mux, aid, token := newHandler(t, spy)

	// K1 claims position 0.
	if code := uploadJPEG(t, mux, aid, token, "K1", 0); code != 200 && code != 201 {
		t.Fatalf("first upload: %d", code)
	}

	// K2 at the same position must fail with 412 and the orphan key must be cleaned up.
	if code := uploadJPEG(t, mux, aid, token, "K2", 0); code != 412 {
		t.Fatalf("expected 412 for position conflict, got %d", code)
	}
	spy.mu.Lock()
	deleted := len(spy.deleteKeys)
	spy.mu.Unlock()
	if deleted == 0 {
		t.Fatal("expected store.Delete to be called for the orphan key after ErrPositionTaken")
	}
}
