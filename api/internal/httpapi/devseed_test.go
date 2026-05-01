package httpapi_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"local/art-web/api/internal/httpapi"
)

func TestDevSeed_DisabledByDefault(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(testDeps(t, "prod")))
	defer srv.Close()
	r, _ := srv.Client().Post(srv.URL+"/dev/seed", "", nil)
	if r.StatusCode != 404 {
		t.Fatalf("dev seed must be 404 outside test env; got %d", r.StatusCode)
	}
}

func TestDevSeed_TestEnv_ReturnsFixture(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(testDeps(t, "test")))
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
}

func TestDevSeed_Many_BulkSeedsPublic(t *testing.T) {
	srv := httptest.NewServer(httpapi.New(testDeps(t, "test")))
	defer srv.Close()
	r, _ := srv.Client().Post(srv.URL+"/dev/seed?many=5", "", nil)
	if r.StatusCode != 200 {
		t.Fatalf("many seed: %d", r.StatusCode)
	}
	feed, _ := srv.Client().Get(srv.URL + "/artworks?limit=10")
	if feed.StatusCode != 200 {
		t.Fatalf("feed: %d", feed.StatusCode)
	}
}
