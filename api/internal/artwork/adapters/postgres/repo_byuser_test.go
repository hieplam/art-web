// api/internal/artwork/adapters/postgres/repo_byuser_test.go
package postgres_test

import (
	"testing"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
)

func TestListByUser_OwnerSeesAll_StrangerSeesPublicOnly(t *testing.T) {
	repo, _, uid := newCtx(t)
	pub, _ := repo.Create(t.Context(), uid, "p", "", "private")
	_ = repo.PatchVisibility(t.Context(), pub, "public")
	_, _ = repo.Create(t.Context(), uid, "draft", "", "private")

	asOwner, _ := repo.ListByUser(t.Context(), uid, true, artworkpostgres.FeedCursor{}, 10)
	if len(asOwner.Items) != 2 {
		t.Fatalf("owner should see 2, got %d", len(asOwner.Items))
	}
	asStranger, _ := repo.ListByUser(t.Context(), uid, false, artworkpostgres.FeedCursor{}, 10)
	if len(asStranger.Items) != 1 {
		t.Fatalf("stranger should see 1, got %d", len(asStranger.Items))
	}
}
