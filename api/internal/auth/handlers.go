// api/internal/auth/handlers.go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type UserUpserter interface {
	UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatarURL string) (userID string, err error)
}

type CookieOpts struct {
	Domain string
	Secure bool
}

// setRawCookie writes a Set-Cookie header directly so the Domain attribute
// preserves the leading dot (e.g. ".example.com") that http.SetCookie strips.
func setRawCookie(w http.ResponseWriter, name, value, domain, path, sameSite string, maxAge int, httpOnly, secure bool) {
	v := fmt.Sprintf("%s=%s; Path=%s", name, value, path)
	if domain != "" {
		v += "; Domain=" + domain
	}
	if maxAge > 0 {
		v += fmt.Sprintf("; Max-Age=%d", maxAge)
	} else if maxAge < 0 {
		v += "; Max-Age=0"
	}
	if httpOnly {
		v += "; HttpOnly"
	}
	if secure {
		v += "; Secure"
	}
	if sameSite != "" {
		v += "; SameSite=" + sameSite
	}
	w.Header().Add("Set-Cookie", v)
}

func StartHandler(providers map[string]Provider, _ string, cookieOpts CookieOpts) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := providers[chi.URLParam(r, "provider")]
		if !ok {
			http.Error(w, `{"error":"unknown_provider"}`, 404)
			return
		}
		state := randState()
		setRawCookie(w, "oauth_state", state, "", "/", "Lax", 600, true, cookieOpts.Secure)
		http.Redirect(w, r, p.AuthURL(state), http.StatusFound)
	})
}

func CallbackHandler(
	providers map[string]Provider,
	users UserUpserter,
	jwts *JWT,
	frontendHome string,
	cookieOpts CookieOpts,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := providers[chi.URLParam(r, "provider")]
		if !ok {
			http.Error(w, `{"error":"unknown_provider"}`, 404)
			return
		}
		stateCookie, err := r.Cookie("oauth_state")
		if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
			http.Error(w, `{"error":"bad_state"}`, 400)
			return
		}
		// Clear the state cookie.
		setRawCookie(w, "oauth_state", "", "", "/", "Lax", -1, true, cookieOpts.Secure)

		prof, err := p.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, `{"error":"exchange_failed"}`, 502)
			return
		}
		uid, err := users.UpsertOAuth(r.Context(), p.Name(), prof.Subject, prof.Email, prof.DisplayName, prof.AvatarURL)
		if err != nil {
			http.Error(w, `{"error":"upsert_failed"}`, 500)
			return
		}
		tok, err := jwts.Issue(uid, 7*24*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"sign_failed"}`, 500)
			return
		}
		setRawCookie(w, "auth", tok, cookieOpts.Domain, "/", "Lax", 7*24*3600, true, cookieOpts.Secure)
		http.Redirect(w, r, frontendHome, http.StatusFound)
	})
}

func LogoutHandler(cookieOpts CookieOpts) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		setRawCookie(w, "auth", "", cookieOpts.Domain, "/", "Lax", -1, true, cookieOpts.Secure)
		w.WriteHeader(http.StatusNoContent)
	})
}

func randState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
