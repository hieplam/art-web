package server_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"local/art-web/api/internal/infrastructure/database"
	infratest "local/art-web/api/internal/infrastructure/testing"
)

func TestDevSeed_DisabledByDefault(t *testing.T) {
	srv := httptest.NewServer(testDeps(t, "prod").router)
	defer srv.Close()
	r, _ := srv.Client().Post(srv.URL+"/dev/seed", "", nil)
	if r.StatusCode != 404 {
		t.Fatalf("dev seed must be 404 outside test env; got %d", r.StatusCode)
	}
}

func TestDevSeed_TestEnv_ReturnsFixture(t *testing.T) {
	srv := httptest.NewServer(testDeps(t, "test").router)
	defer srv.Close()

	r, err := srv.Client().Post(srv.URL+"/dev/seed", "application/json", nil)
	if err != nil || r.StatusCode != 200 {
		t.Fatalf("seed POST: status %d err %v", r.StatusCode, err)
	}
	var out struct {
		AliceCookie, BobCookie string
		AliceSlug, BobSlug     string
		PID                    string `json:"pId"`
		QID                    string `json:"qId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(out.AliceCookie, "auth=") || !strings.HasPrefix(out.BobCookie, "auth=") {
		t.Fatalf("cookies must be auth=...; got alice=%q bob=%q", out.AliceCookie, out.BobCookie)
	}
	if out.PID == "" || out.QID == "" || out.PID == out.QID {
		t.Fatalf("PID/QID must be distinct non-empty UUIDs; got P=%q Q=%q", out.PID, out.QID)
	}

	pool, err := database.New(t.Context(), infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	rows, err := pool.Query(t.Context(), `
		SELECT id, storage_key
		FROM artwork_images
		WHERE artwork_id IN ($1, $2)
		ORDER BY artwork_id`, out.PID, out.QID)
	if err != nil {
		t.Fatalf("query seed images: %v", err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			t.Fatalf("scan seed image: %v", err)
		}
		if !strings.Contains(key, "/"+id+".") {
			t.Fatalf("seed storage_key %q does not include image id %q", key, id)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("seed image rows: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 seed image rows, got %d", n)
	}
}

func TestDevSeed_Many_BulkSeedsPublic(t *testing.T) {
	srv := httptest.NewServer(testDeps(t, "test").router)
	defer srv.Close()
	r, _ := srv.Client().Post(srv.URL+"/dev/seed?many=5", "", nil)
	if r.StatusCode != 200 {
		t.Fatalf("many seed: %d", r.StatusCode)
	}
	feed, _ := srv.Client().Get(srv.URL + "/artworks?limit=10")
	if feed.StatusCode != 200 {
		t.Fatalf("feed: %d", feed.StatusCode)
	}
	var body struct {
		Items []any `json:"items"`
	}
	if err := json.NewDecoder(feed.Body).Decode(&body); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if len(body.Items) == 0 {
		t.Fatal("feed must contain seeded bulk items")
	}
}

func TestDevSeed_Many120_WritesAllItems(t *testing.T) {
	httptest.NewServer(testDeps(t, "test").router) // ensures DB + schema are ready
	srv := httptest.NewServer(testDeps(t, "test").router)
	defer srv.Close()

	r, _ := srv.Client().Post(srv.URL+"/dev/seed?many=120", "", nil)
	if r.StatusCode != 200 {
		t.Fatalf("many=120 seed: %d", r.StatusCode)
	}

	// Query the DB directly to confirm all 120 bulk items (+ 1 public P) were written,
	// not silently capped at the old maxMany=100 limit.
	pool, err := database.New(t.Context(), infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	var n int
	_ = pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM artworks WHERE visibility='public'`).Scan(&n)
	if n < 121 {
		t.Fatalf("expected at least 121 public artworks (120 bulk + 1 P), got %d — cap may still be too low", n)
	}
}

func TestDevSeed_NotMounted_When_AppEnv_NotTest(t *testing.T) {
	deps := testDeps(t, "production") // helper from testutil_test.go

	req := httptest.NewRequest("POST", "/dev/seed", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("expected /dev/seed to be unmounted in non-test env; status=%d body=%s",
			rec.Code, rec.Body.String())
	}
}

func TestDevSeed_FixedSuffix_ProducesDeterministicSlug(t *testing.T) {
	deps := testDeps(t, "test") // helper from testutil_test.go

	req := httptest.NewRequest("POST", "/dev/seed?suffix=fixed1234", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		AliceSlug string `json:"aliceSlug"`
		BobSlug   string `json:"bobSlug"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AliceSlug != "alice-fixed1234" {
		t.Fatalf("alice slug=%q want alice-fixed1234", resp.AliceSlug)
	}
	if resp.BobSlug != "bob-fixed1234" {
		t.Fatalf("bob slug=%q want bob-fixed1234", resp.BobSlug)
	}
}
