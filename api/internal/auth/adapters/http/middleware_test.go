// api/internal/auth/adapters/http/middleware_test.go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "local/art-web/api/internal/auth/adapters/http"
	authservice "local/art-web/api/internal/auth/service"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestMiddleware_NoCookie_StillCallsNext(t *testing.T) {
	j := authservice.NewJWT(testKey, nil)
	mw := authhttp.Middleware(j)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := authhttp.UserIDFrom(r.Context()); ok {
			t.Fatalf("expected no user, got %s", uid)
		}
		called = true
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("next never called")
	}
}

func TestMiddleware_ValidCookie_PopulatesContext(t *testing.T) {
	j := authservice.NewJWT(testKey, nil)
	tok, _ := j.Issue("uid-42", time.Hour)
	mw := authhttp.Middleware(j)
	var got string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := authhttp.UserIDFrom(r.Context())
		got = uid
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "uid-42" {
		t.Fatalf("uid = %s", got)
	}
}

func TestRequireUser_401WhenAbsent(t *testing.T) {
	h := authhttp.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 401 {
		t.Fatalf("code = %d", rec.Code)
	}
}
