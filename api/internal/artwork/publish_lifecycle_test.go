package artwork_test

import "testing"

func TestPublishLifecycle_Case21_PublishedAtStableAfterFirstFlip(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, err := repo.Create(t.Context(), uid, "x", "", "private")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	a, err := repo.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get(initial): %v", err)
	}
	if a.PublishedAt != nil {
		t.Fatal("draft should have NULL published_at")
	}

	// First publish.
	if err := repo.PatchVisibility(t.Context(), id, "public"); err != nil {
		t.Fatalf("PatchVisibility(public): %v", err)
	}
	a, err = repo.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get(after publish): %v", err)
	}
	if a.PublishedAt == nil {
		t.Fatal("first publish should set published_at")
	}
	first := *a.PublishedAt

	// Cycle private→public→private→public.
	for _, v := range []string{"private", "public", "private", "public"} {
		if err := repo.PatchVisibility(t.Context(), id, v); err != nil {
			t.Fatalf("PatchVisibility(%s): %v", v, err)
		}
	}
	a, err = repo.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get(final): %v", err)
	}
	if !a.PublishedAt.Equal(first) {
		t.Fatalf("published_at drifted: was %v now %v (%v elapsed)",
			first, *a.PublishedAt, a.PublishedAt.Sub(first))
	}
}
