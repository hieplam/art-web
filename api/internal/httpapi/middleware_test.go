package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"local/art-web/api/internal/httpapi"
)

func TestCORS_AllowsCredentialsForAllowedOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/me", nil)
	req.Header.Set("Origin", "https://app.example.com")
	httpapi.CORSFor("https://app.example.com")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal()
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatal()
	}
}

func TestCacheControl_PublicFeedIsShortPublic(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := httpapi.CacheControlByPath()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/artworks", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Fatalf("/artworks Cache-Control=%q want public, max-age=60", got)
	}
}

func TestCacheControl_ArtworkDetailIsPrivateNoStore(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := httpapi.CacheControlByPath()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/artworks/abc-123", nil))
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("/artworks/<id> Cache-Control=%q want private, no-store", got)
	}
}

func TestCacheControl_MeIsPrivateNoStore(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := httpapi.CacheControlByPath()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("/me Cache-Control=%q want private, no-store", got)
	}
}

func TestCacheControl_TagListingIsShortPublic(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := httpapi.CacheControlByPath()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/tags/landscape", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Fatalf("/tags/<name> Cache-Control=%q want public, max-age=60", got)
	}
}
