// api/internal/artwork/repo_test.go
package artwork_test

import (
	"context"
	"testing"
	"time"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/user"
)

func newCtx(t *testing.T) (*artwork.Repo, *user.Repo, string) {
	pool, err := db.New(context.Background(), dbtest.StartPostgres(t))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})
	users := user.NewRepo(pool)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	return artwork.NewRepo(pool), users, uid
}

func TestCreate_DefaultsToPrivateNullPublishedAt(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, err := repo.Create(t.Context(), uid, "Hello", "", "private")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := repo.Get(t.Context(), id)
	if got.Visibility != "private" {
		t.Fatal()
	}
	if got.PublishedAt != nil {
		t.Fatalf("published_at should be NULL, got %v", got.PublishedAt)
	}
}

func TestPatchVisibility_SetsPublishedAtOnFirstPublic(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := repo.PatchVisibility(t.Context(), id, "public"); err != nil {
		t.Fatalf("patch: %v", err)
	}
	got, _ := repo.Get(t.Context(), id)
	if got.PublishedAt == nil || time.Since(*got.PublishedAt) > time.Minute {
		t.Fatalf("published_at not set: %v", got.PublishedAt)
	}
	stamp := *got.PublishedAt
	_ = repo.PatchVisibility(t.Context(), id, "private")
	_ = repo.PatchVisibility(t.Context(), id, "public")
	got2, _ := repo.Get(t.Context(), id)
	if !got2.PublishedAt.Equal(stamp) {
		t.Fatalf("published_at changed on republish: %v vs %v", *got2.PublishedAt, stamp)
	}
}

func TestDelete_CascadesImagesAndTags(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := repo.Delete(t.Context(), id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(t.Context(), id); err == nil {
		t.Fatal("expected not-found after delete")
	}
}

func TestPatchTitle_PreservesDescription(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "title-1", "important description", "private")
	if err := repo.PatchTitle(t.Context(), id, "title-2"); err != nil {
		t.Fatalf("patch title: %v", err)
	}
	got, _ := repo.Get(t.Context(), id)
	if got.Title != "title-2" {
		t.Fatalf("title not updated: %q", got.Title)
	}
	if got.Description == nil || *got.Description != "important description" {
		t.Fatalf("description was clobbered: %v", got.Description)
	}
}

func TestPatchDescription_PreservesTitle(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "stable-title", "old", "private")
	if err := repo.PatchDescription(t.Context(), id, "new description"); err != nil {
		t.Fatalf("patch desc: %v", err)
	}
	got, _ := repo.Get(t.Context(), id)
	if got.Title != "stable-title" {
		t.Fatalf("title was clobbered: %q", got.Title)
	}
	if got.Description == nil || *got.Description != "new description" {
		t.Fatalf("description not updated: %v", got.Description)
	}
}

func TestSetCoverPosition_PersistsValue(t *testing.T) {
	repo, _, uid := newCtx(t)
	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")

	if err := repo.SetCoverPosition(t.Context(), aid, 5); err != nil {
		t.Fatalf("SetCoverPosition: %v", err)
	}
	got, _ := repo.Get(t.Context(), aid)
	if got.CoverPosition != 5 {
		t.Fatalf("got %d want 5", got.CoverPosition)
	}
}
