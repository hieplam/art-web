// api/internal/auth/adapters/http/flow_test.go
//
// Integration-level tests that exercise the OAuth handlers with the *real*
// googleProvider (via NewGoogleProviderForTest). The unit tests in
// handlers_test.go use a hand-rolled fakeProvider that bypasses
// oauth2.Config.AuthCodeURL — so they cannot catch a misconfigured client_id.
// These tests can.
package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	authhttp "local/art-web/api/internal/auth/adapters/http"
	authoauth "local/art-web/api/internal/auth/adapters/oauth"
	authports "local/art-web/api/internal/auth/ports"
	authservice "local/art-web/api/internal/auth/service"
)

const (
	testClientID     = "test-client-id-abc123.apps.googleusercontent.com"
	testClientSecret = "test-client-secret"
	testRedirectURL  = "http://api.local/auth/google/callback"
)

// fakeGoogleServer returns an httptest.Server that stands in for Google's
// /token and /userinfo endpoints with deterministic responses.
func fakeGoogleServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "fake-access-token",
				"id_token":     "x.y.z",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":     "google-sub-42",
				"email":   "alice@example.com",
				"name":    "Alice Anderson",
				"picture": "http://avatar.example.com/a.png",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestStart_RealProvider_IncludesClientIDInRedirect(t *testing.T) {
	srv := fakeGoogleServer(t)
	defer srv.Close()

	provider := authoauth.NewGoogleProviderForTest(
		testClientID, testClientSecret, testRedirectURL,
		"https://accounts.google.com/o/oauth2/v2/auth", // real authorize URL — we never hit it
		srv.URL+"/token",
		srv.URL+"/userinfo",
	)
	h := authhttp.StartHandler(map[string]authports.Provider{"google": provider}, authhttp.CookieOpts{})

	req := withChiURLParam(httptest.NewRequest("GET", "/auth/google/start", nil), "provider", "google")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location not a URL: %v", err)
	}

	if got := loc.Query().Get("client_id"); got != testClientID {
		t.Fatalf("client_id mismatch — bug regression?\n  want: %q\n  got:  %q", testClientID, got)
	}
	if got := loc.Query().Get("redirect_uri"); got != testRedirectURL {
		t.Fatalf("redirect_uri mismatch:\n  want: %q\n  got:  %q", testRedirectURL, got)
	}
	if got := loc.Query().Get("response_type"); got != "code" {
		t.Fatalf("response_type = %q, want %q", got, "code")
	}
	if got := loc.Query().Get("scope"); got != "openid email profile" {
		t.Fatalf("scope = %q, want %q", got, "openid email profile")
	}
	if loc.Query().Get("state") == "" {
		t.Fatal("state param missing in Location URL")
	}
	if c := getCookie(rec.Result(), "oauth_state"); c == nil || c.Value == "" {
		t.Fatal("oauth_state cookie not set")
	}
}

func TestFullOAuthFlow_StartThenCallback(t *testing.T) {
	srv := fakeGoogleServer(t)
	defer srv.Close()

	provider := authoauth.NewGoogleProviderForTest(
		testClientID, testClientSecret, testRedirectURL,
		srv.URL+"/auth", // unused — Start emits a Location but we don't follow it
		srv.URL+"/token",
		srv.URL+"/userinfo",
	)
	repo := &fakeUserRepo{}
	jwts := authservice.NewJWT(testKey, nil)
	cookieOpts := authhttp.CookieOpts{Secure: false}
	providers := map[string]authports.Provider{"google": provider}

	// --- Phase 1: GET /auth/google/start ---------------------------------
	startH := authhttp.StartHandler(providers, cookieOpts)
	startReq := withChiURLParam(httptest.NewRequest("GET", "/auth/google/start", nil), "provider", "google")
	startRec := httptest.NewRecorder()
	startH.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusFound {
		t.Fatalf("start: expected 302, got %d", startRec.Code)
	}
	stateCookie := getCookie(startRec.Result(), "oauth_state")
	if stateCookie == nil {
		t.Fatal("start: oauth_state cookie not set")
	}
	loc, _ := url.Parse(startRec.Header().Get("Location"))
	stateInURL := loc.Query().Get("state")
	if stateInURL == "" || stateInURL != stateCookie.Value {
		t.Fatalf("start: state param/cookie mismatch (url=%q cookie=%q)", stateInURL, stateCookie.Value)
	}

	// --- Phase 2: GET /auth/google/callback?code=...&state=... -----------
	// Simulate Google redirecting back with the same state and a fresh code.
	callbackH := authhttp.CallbackHandler(providers, repo, jwts, "https://app.local/", cookieOpts)
	cbURL := "/auth/google/callback?code=fake-auth-code&state=" + stateInURL
	cbReq := withChiURLParam(httptest.NewRequest("GET", cbURL, nil), "provider", "google")
	cbReq.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateCookie.Value})
	cbRec := httptest.NewRecorder()
	callbackH.ServeHTTP(cbRec, cbReq)

	if cbRec.Code != http.StatusFound {
		t.Fatalf("callback: expected 302, got %d (body: %q)", cbRec.Code, cbRec.Body.String())
	}
	if got := cbRec.Header().Get("Location"); got != "https://app.local/" {
		t.Fatalf("callback: Location = %q, want frontendHome", got)
	}

	// --- Phase 3: assert the auth cookie carries a valid JWT -------------
	authCookie := getCookie(cbRec.Result(), "auth")
	if authCookie == nil || authCookie.Value == "" {
		t.Fatal("callback: auth cookie not set")
	}
	if !authCookie.HttpOnly {
		t.Error("callback: auth cookie missing HttpOnly")
	}
	if authCookie.MaxAge != 7*24*3600 {
		t.Errorf("callback: auth cookie MaxAge = %d, want %d", authCookie.MaxAge, 7*24*3600)
	}

	uid, err := jwts.Verify(authCookie.Value)
	if err != nil {
		t.Fatalf("callback: issued JWT does not verify: %v", err)
	}
	if !strings.HasPrefix(uid, "uid-google-sub-42") {
		t.Errorf("callback: JWT subject = %q, want prefix %q (came from /userinfo sub)", uid, "uid-google-sub-42")
	}
	if email, ok := repo.stored[uid]; !ok || email != "alice@example.com" {
		t.Errorf("callback: user not upserted with email from /userinfo (stored=%v)", repo.stored)
	}

	// --- Phase 4: state cookie must be cleared after consumption ---------
	clearedState := getCookie(cbRec.Result(), "oauth_state")
	if clearedState == nil || clearedState.MaxAge != -1 {
		t.Errorf("callback: oauth_state cookie not cleared (got %+v)", clearedState)
	}
}
