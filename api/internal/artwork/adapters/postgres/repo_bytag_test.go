// api/internal/artwork/adapters/postgres/repo_bytag_test.go
package postgres_test

import (
	"testing"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
)

func TestListByTag_PublicOnly(t *testing.T) {
	repo, _, uid := newCtx(t)
	tags := artworkpostgres.NewTagsRepo(repo.DB())

	pub, _ := repo.Create(t.Context(), uid, "tagged public", "", "private")
	_ = repo.PatchVisibility(t.Context(), pub, "public")
	_ = tags.SetTags(t.Context(), pub, []string{"cats"})

	priv, _ := repo.Create(t.Context(), uid, "tagged private", "", "private")
	_ = tags.SetTags(t.Context(), priv, []string{"cats"})

	page, err := repo.ListByTag(t.Context(), "cats", artworkpostgres.FeedCursor{}, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 public item, got %d", len(page.Items))
	}
	if page.Items[0].ID != pub {
		t.Fatalf("wrong item: %s", page.Items[0].ID)
	}
}
