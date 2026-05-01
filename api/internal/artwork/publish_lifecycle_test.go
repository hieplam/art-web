package artwork_test

import (
	"testing"
	"time"
)

func TestPublishLifecycle_Case21_PublishedAtStableAfterFirstFlip(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")

	a, _ := repo.Get(t.Context(), id)
	if a.PublishedAt != nil {
		t.Fatal("draft should have NULL published_at")
	}

	// First publish.
	_ = repo.PatchVisibility(t.Context(), id, "public")
	a, _ = repo.Get(t.Context(), id)
	if a.PublishedAt == nil {
		t.Fatal("first publish should set published_at")
	}
	first := *a.PublishedAt

	// Cycle private→public→private→public.
	for _, v := range []string{"private", "public", "private", "public"} {
		_ = repo.PatchVisibility(t.Context(), id, v)
	}
	a, _ = repo.Get(t.Context(), id)
	if !a.PublishedAt.Equal(first) {
		t.Fatalf("published_at drifted: was %v now %v (%v elapsed)",
			first, *a.PublishedAt, a.PublishedAt.Sub(first))
	}
	_ = time.Now // pin import
}
