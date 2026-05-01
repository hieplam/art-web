// api/internal/artwork/tags_test.go
package artwork_test

import (
	"testing"

	"local/art-web/api/internal/artwork"
)

func TestUpsertTags_NormalizesAndDedupes(t *testing.T) {
	repo, _, uid := newCtx(t)
	tags := artwork.NewTagsRepo(repo.Pool())
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := tags.SetTags(t.Context(), id, []string{"  Cats  ", "cats", "DOGS"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := tags.GetTags(t.Context(), id)
	if len(got) != 2 {
		t.Fatalf("dedup failed: %v", got)
	}
}
