package image_test

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
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
)

type failingStore struct {
	storage.Storage
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

func newHandler(t *testing.T, store storage.Storage) (*chi.Mux, string, string) {
	pool, err := db.New(context.Background(), dbtest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql); return err
	})
	users := user.NewRepo(pool)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S", "a@b", "alice", "")
	arts := artwork.NewRepo(pool)
	aid, _ := arts.Create(t.Context(), uid, "x", "", "private")

	images := image.NewRepo(pool)
	svc := image.NewService(store, images, arts)
	jwts := auth.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil)
	h := image.NewHandler(svc, arts, auth.NewURLBuilder("http://x", []byte("k"), nil))

	r := chi.NewRouter()
	r.Use(auth.Middleware(jwts))
	r.With(auth.RequireUser).Post("/artworks/{id}/images", h.Upload)
	tok, _ := jwts.Issue(uid, 3600*time.Second)
	return r, aid, tok
}

func uploadJPEG(t *testing.T, mux http.Handler, artID, token, clientID string, position int) int {
	t.Helper()
	raw, _ := os.ReadFile("testdata/sample.jpg")

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	manifest := fmt.Sprintf(`[{"client_image_id":"%s","position":%d,"content_type":"image/jpeg"}]`, clientID, position)
	_ = mw.WriteField("manifest", manifest)
	w, _ := mw.CreateFormFile("files", "f.jpg")
	_, _ = w.Write(raw)
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+artID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestUploadCase19_ConcurrentSamePosition(t *testing.T) {
	store := storage.NewLocalFS(t.TempDir())
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
	base := storage.NewLocalFS(t.TempDir())
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
	if rec2.Code != 200 {
		t.Fatalf("retry status %d", rec2.Code)
	}

	// dbtest.StartPostgres is sync.Once: the second call returns the same DSN as
	// newHandler used, so this pool reads the same database. Tests must not run
	// in parallel with other tests that call TruncateAll on the shared container.
	pool, err := db.New(t.Context(), dbtest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	var n int
	_ = pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM artwork_images WHERE artwork_id=$1`, aid).Scan(&n)
	if n != 5 {
		t.Fatalf("expected 5 rows, got %d", n)
	}
}
