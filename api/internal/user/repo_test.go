// api/internal/user/repo_test.go
package user_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/user"
)

func newRepo(t *testing.T) *user.Repo {
	pool, err := db.New(context.Background(), dbtest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
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

func TestUpsertOAuth_SlugExhaustionAfter50Collisions(t *testing.T) {
	r := newRepo(t)
	// Seed 50 users that occupy the slug space "alice", "alice-2", ..., "alice-50".
	for i := 0; i < 50; i++ {
		subject := "S" + strings.Repeat("x", i+1) // unique oauth_subject per insertion
		if _, err := r.UpsertOAuth(t.Context(), "google", subject, "x@x", "alice", ""); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// 51st upsert must run out of slug space and return the sentinel error.
	_, err := r.UpsertOAuth(t.Context(), "google", "exhausted-subject", "x@x", "alice", "")
	if err == nil {
		t.Fatal("expected slug-exhaustion error")
	}
	if !strings.Contains(err.Error(), "slug exhausted") {
		t.Fatalf("expected 'slug exhausted', got %v", err)
	}
}

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	r := newRepo(t)
	_, err := r.Get(t.Context(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetBySlug_NotFound_PassesThroughError(t *testing.T) {
	r := newRepo(t)
	_, err := r.GetBySlug(t.Context(), "no-such-slug")
	if err == nil {
		t.Fatal("expected error for missing slug")
	}
	// Current behavior: GetBySlug does NOT translate to ErrNotFound — it returns
	// the raw pgx.ErrNoRows. This test pins that behavior so it does not change
	// silently.
	if errors.Is(err, user.ErrNotFound) {
		t.Fatalf("GetBySlug should not return ErrNotFound; got %v", err)
	}
}

func TestUpsertOAuth_DuplicateOAuthKey_ReturnsExistingID(t *testing.T) {
	r := newRepo(t)

	first, err := r.UpsertOAuth(t.Context(), "google", "S-DUP", "a@b", "alice-dup", "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Re-upsert with the SAME provider+subject but different display name → must
	// hit the lookupExistingOAuth fast path at repo.go:45-55 (UPDATE, no INSERT).
	second, err := r.UpsertOAuth(t.Context(), "google", "S-DUP", "a@b", "different-name", "")
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if first != second {
		t.Fatalf("expected same id; got %s vs %s", first, second)
	}
}

func TestGetBySlug_FoundReturnsUser(t *testing.T) {
	r := newRepo(t)
	uid, err := r.UpsertOAuth(t.Context(), "google", "S-FIND", "f@b", "findable", "")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := r.GetBySlug(t.Context(), "findable")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got.ID != uid {
		t.Fatalf("got id %q want %q", got.ID, uid)
	}
	if got.Slug != "findable" {
		t.Fatalf("got slug %q want %q", got.Slug, "findable")
	}
}
