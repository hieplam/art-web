// api/internal/auth/handlers_test.go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"local/art-web/api/internal/auth"
)

type fakeProvider struct{}

func (fakeProvider) Name() string                { return "google" }
func (fakeProvider) AuthURL(state string) string { return "https://goog/auth?state=" + state }
func (fakeProvider) Exchange(_ context.Context, code string) (*auth.Profile, error) {
	return &auth.Profile{Subject: "S-" + code, Email: "a@b", DisplayName: "Alice", AvatarURL: ""}, nil
}

type fakeUserRepo struct{ stored map[string]string }

func (f *fakeUserRepo) UpsertOAuth(_ context.Context, prov, sub, email, name, avatar string) (string, error) {
	if f.stored == nil {
		f.stored = map[string]string{}
	}
	uid := "uid-" + sub
	f.stored[uid] = email
	return uid, nil
}

func TestStart_RedirectsToProviderWithState(t *testing.T) {
	h := auth.StartHandler(map[string]auth.Provider{"google": fakeProvider{}}, auth.CookieOpts{Secure: true})
	rec := httptest.NewRecorder()
	req := withChiURLParam(httptest.NewRequest("GET", "/auth/google/start", nil), "provider", "google")
	h.ServeHTTP(rec, req)
	if rec.Code != 302 {
		t.Fatalf("code %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if !strings.Contains(loc.String(), "state=") {
		t.Fatal("no state param")
	}
	if c := getCookie(rec.Result(), "oauth_state"); c == nil {
		t.Fatal("no state cookie")
	}
}

func TestCallback_SetsAuthCookie(t *testing.T) {
	repo := &fakeUserRepo{}
	jwts := auth.NewJWT(testKey, nil)
	opts := auth.CookieOpts{Domain: ".example.com", Secure: true}
	h := auth.CallbackHandler(map[string]auth.Provider{"google": fakeProvider{}}, repo, jwts, "https://app.example.com/", opts)
	req := httptest.NewRequest("GET", "/auth/google/callback?code=C&state=S", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "S"})
	req = withChiURLParam(req, "provider", "google")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 302 {
		t.Fatalf("code %d", rec.Code)
	}
	c := getCookie(rec.Result(), "auth")
	if c == nil || c.Value == "" {
		t.Fatal("auth cookie not set")
	}
	if !c.HttpOnly || !c.Secure || c.Domain != ".example.com" {
		t.Fatalf("cookie attributes wrong: %+v", c)
	}
}

func TestLogout_ClearsAuthCookie(t *testing.T) {
	h := auth.LogoutHandler(auth.CookieOpts{Domain: ".example.com", Secure: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/auth/logout", nil))
	c := getCookie(rec.Result(), "auth")
	if c == nil || c.MaxAge != -1 || c.Domain != ".example.com" {
		t.Fatalf("cookie not cleared correctly: %+v", c)
	}
}

func withChiURLParam(r *http.Request, k, v string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(k, v)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func getCookie(r *http.Response, name string) *http.Cookie {
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
