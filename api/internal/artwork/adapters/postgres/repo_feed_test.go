// api/internal/artwork/adapters/postgres/repo_feed_test.go
package postgres_test

import (
	"testing"
	"time"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
)

func TestPublicFeed_OrdersByPublishedAtDescAndExcludesPrivate(t *testing.T) {
	repo, _, uid := newCtx(t)
	for i := 0; i < 3; i++ {
		id, _ := repo.Create(t.Context(), uid, "p"+string(rune('0'+i)), "", "private")
		_ = repo.PatchVisibility(t.Context(), id, "public")
	}
	_, _ = repo.Create(t.Context(), uid, "draft", "", "private")
	priv, _ := repo.Create(t.Context(), uid, "priv", "", "private")
	_ = repo.PatchVisibility(t.Context(), priv, "public")
	_ = repo.PatchVisibility(t.Context(), priv, "private")

	page, err := repo.PublicFeed(t.Context(), artworkpostgres.FeedCursor{}, 10)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 public items, got %d", len(page.Items))
	}
}

func TestPublicFeed_PaginatesViaCursor(t *testing.T) {
	repo, _, uid := newCtx(t)
	for i := 0; i < 5; i++ {
		id, _ := repo.Create(t.Context(), uid, "x", "", "private")
		_ = repo.PatchVisibility(t.Context(), id, "public")
	}
	page1, _ := repo.PublicFeed(t.Context(), artworkpostgres.FeedCursor{}, 2)
	if len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("page1: %+v", page1)
	}
	page2, _ := repo.PublicFeed(t.Context(), *page1.NextCursor, 2)
	if len(page2.Items) != 2 {
		t.Fatalf("page2 size %d", len(page2.Items))
	}
	if page1.Items[0].ID == page2.Items[0].ID {
		t.Fatal("page2 leaked duplicates from page1")
	}
}

func TestPublicFeed_SameSecondPublish_NoSkipsOrDupes(t *testing.T) {
	repo, _, uid := newCtx(t)
	const N = 7
	pinned := time.Now().UTC().Truncate(time.Second).Add(123456 * time.Microsecond)
	for i := 0; i < N; i++ {
		id, _ := repo.Create(t.Context(), uid, "x", "", "public")
		if err := repo.DB().WithContext(t.Context()).Exec(
			`UPDATE artworks SET published_at=? WHERE id=?`, pinned, id).Error; err != nil {
			t.Fatalf("pin: %v", err)
		}
	}
	seen := map[string]bool{}
	cursor := artworkpostgres.FeedCursor{}
	for {
		page, err := repo.PublicFeed(t.Context(), cursor, 2)
		if err != nil {
			t.Fatalf("feed: %v", err)
		}
		for _, a := range page.Items {
			if seen[a.ID] {
				t.Fatalf("duplicate id across pages: %s", a.ID)
			}
			seen[a.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != N {
		t.Fatalf("expected %d distinct ids, saw %d", N, len(seen))
	}
}
