// api/internal/auth/adapters/http/me_integration_test.go
//
// /me happy-path coverage for the per-slice gate. The unit tests in
// router_test.go exercise the no-cookie 401 branch with a nil user repo;
// this file pairs with a real testcontainers Postgres so the renderUser
// branch and the user-found path are reachable.
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	authhttp "local/art-web/api/internal/auth/adapters/http"
	authports "local/art-web/api/internal/auth/ports"
	authservice "local/art-web/api/internal/auth/service"
	"local/art-web/api/internal/infrastructure/database"
	infratest "local/art-web/api/internal/infrastructure/testing"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

// TestMe_HappyPath_RendersUser asserts /me with a valid auth cookie returns
// 200 with the renderUser shape (id, slug, display_name, avatar_url keys).
func TestMe_HappyPath_RendersUser(t *testing.T) {
	pool, err := database.New(context.Background(), infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	infratest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})

	users := userpostgres.NewRepo(pool)
	uid, err := users.UpsertOAuth(context.Background(), "google", "S-me", "alice@me", "Alice", "")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	jwts := authservice.NewJWT(testKey, nil)
	mw := authhttp.NewMiddleware(jwts)
	h := authhttp.NewHandler(authhttp.AuthDeps{
		Providers:    map[string]authports.OAuthProvider{},
		Users:        users,
		JWT:          jwts,
		FrontendHome: authhttp.FrontendHome("http://frontend.test/"),
		CookieOpts:   authhttp.CookieOpts{Secure: false},
	})
	router := authhttp.NewRouter(h)

	r := chi.NewRouter()
	r.Use(mw.ParseToken())
	router.RegisterRoutes(r)

	tok, _ := jwts.Issue(uid, time.Hour)
	req := httptest.NewRequest("GET", "/me", nil)
	req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id"`) || !strings.Contains(body, `"slug"`) ||
		!strings.Contains(body, `"display_name"`) || !strings.Contains(body, `"avatar_url"`) {
		t.Errorf("/me response missing canonical fields; body=%s", body)
	}
}
