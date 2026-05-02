// api/internal/auth/middleware_test.go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"local/art-web/api/internal/auth"
)

func TestMiddleware_NoCookie_StillCallsNext(t *testing.T) {
	j := auth.NewJWT(testKey, nil)
	mw := auth.Middleware(j)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := auth.UserIDFrom(r.Context()); ok {
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
	j := auth.NewJWT(testKey, nil)
	tok, _ := j.Issue("uid-42", time.Hour)
	mw := auth.Middleware(j)
	var got string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
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
	h := auth.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 401 {
		t.Fatalf("code = %d", rec.Code)
	}
}
