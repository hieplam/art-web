// api/internal/user/repo_test.go
package user_test

import (
	"context"
	"strings"
	"testing"

	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/user"
)

func newRepo(t *testing.T) *user.Repo {
	pool, _ := db.New(context.Background(), dbtest.StartPostgres(t))
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})
	return user.NewRepo(pool)
}

func TestUpsertOAuth_FirstTimeAssignsSlug(t *testing.T) {
	r := newRepo(t)
	uid, err := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "Alice Smith", "")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	u, _ := r.Get(t.Context(), uid)
	if u.Slug == "" {
		t.Fatal("slug not assigned")
	}
}

func TestUpsertOAuth_ConflictingSlugGetsSuffixed(t *testing.T) {
	r := newRepo(t)
	if _, err := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", ""); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	uid2, err := r.UpsertOAuth(t.Context(), "google", "S2", "x@b", "alice", "")
	if err != nil {
		t.Fatalf("second upsert (different subject, same display name): %v", err)
	}
	u2, err := r.Get(t.Context(), uid2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if u2.Slug == "alice" {
		t.Fatal("slug collision not resolved")
	}
	if !strings.HasPrefix(u2.Slug, "alice-") {
		t.Fatalf("expected suffixed slug, got %q", u2.Slug)
	}
}

func TestUpsertOAuth_Idempotent_ReturnsSameID(t *testing.T) {
	r := newRepo(t)
	a, err := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a != b {
		t.Fatalf("upsert produced two ids: %s vs %s", a, b)
	}
}
