package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// /me must distinguish three cases for the cookie-staleness bug fix:
//   - no JWT          -> 401, no Set-Cookie (nothing to clear).
//   - valid JWT, but the subject UUID is not in the users table (e.g. after
//     `dev-up.sh down -v`) -> 401 + Set-Cookie clearing `auth`. The 404 here
//     used to crash Nav.tsx because its catch only swallowed 401.
//   - valid JWT and live user -> 200 with the user payload.
func TestMeHandler_NoCookie_Returns401(t *testing.T) {
	d := testDeps(t, "test")

	rec := httptest.NewRecorder()
	d.router.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("Set-Cookie should not be set when there was no auth cookie; got %q", got)
	}
}

func TestMeHandler_ValidJWTMissingUser_Returns401AndClearsCookie(t *testing.T) {
	d := testDeps(t, "test")

	// Issue a valid JWT for a UUID that does not exist in the users table.
	// Mirrors the dev-up reseed scenario: signature verifies (same key) but
	// the subject was wiped along with the rest of the schema.
	ghostID := uuid.NewString()
	tok, err := d.JWT.Issue(ghostID, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	req := httptest.NewRequest("GET", "/me", nil)
	req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	rec := httptest.NewRecorder()
	d.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"unauthorized"`) {
		t.Errorf("body should contain unauthorized error code; got %s", body)
	}

	// Set-Cookie must be present and zero out the auth cookie. We accept any
	// shape that drops the value to empty and either Max-Age=0 or an Expires
	// in the past — the LogoutHandler convention is Max-Age=0 via setRawCookie.
	sc := rec.Header().Get("Set-Cookie")
	if sc == "" {
		t.Fatalf("Set-Cookie not set; want auth cookie clear directive")
	}
	if !strings.Contains(sc, "auth=") || !strings.Contains(sc, "Max-Age=0") {
		t.Errorf("Set-Cookie should clear auth cookie via Max-Age=0; got %q", sc)
	}
}
