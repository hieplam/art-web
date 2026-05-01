// api/internal/auth/google_test.go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"local/art-web/api/internal/auth"
)

func TestGoogle_Exchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at", "id_token": "x.y.z", "token_type": "Bearer", "expires_in": 3600,
			})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub": "100", "email": "a@b.com", "name": "Alice", "picture": "http://avatar/a.png",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := auth.NewGoogleProviderForTest("cid", "csecret", "http://app/cb", srv.URL+"/auth", srv.URL+"/token", srv.URL+"/userinfo")
	prof, err := p.Exchange(t.Context(), "the-code")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if prof.Subject != "100" || prof.Email != "a@b.com" || prof.DisplayName != "Alice" {
		t.Fatalf("bad profile: %+v", prof)
	}
}
