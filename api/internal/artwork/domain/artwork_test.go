// api/internal/artwork/domain/artwork_test.go
package domain_test

import (
	"errors"
	"testing"

	"local/art-web/api/internal/artwork/domain"
)

// TestPublish_AlreadyPublished asserts an artwork already at public visibility
// returns ErrAlreadyPublished — the sentinel the visibility service catches and
// translates to a 204 no-op (spec §7.2.1).
func TestPublish_AlreadyPublished(t *testing.T) {
	a := &domain.Artwork{Visibility: domain.VisibilityPublic, Images: []domain.Image{{}}}
	err := a.Publish()
	if !errors.Is(err, domain.ErrAlreadyPublished) {
		t.Fatalf("Publish on already-public artwork: err=%v want ErrAlreadyPublished", err)
	}
}

// TestPublish_NoImages asserts publishing an artwork with zero images returns
// ErrNoImages.
func TestPublish_NoImages(t *testing.T) {
	a := &domain.Artwork{Visibility: domain.VisibilityPrivate}
	err := a.Publish()
	if !errors.Is(err, domain.ErrNoImages) {
		t.Fatalf("Publish on imageless artwork: err=%v want ErrNoImages", err)
	}
}

// TestPublish_HappyPath asserts a private artwork with images flips to public
// and returns nil.
func TestPublish_HappyPath(t *testing.T) {
	a := &domain.Artwork{
		Visibility: domain.VisibilityPrivate,
		Images:     []domain.Image{{}},
	}
	if err := a.Publish(); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if a.Visibility != domain.VisibilityPublic {
		t.Fatalf("Visibility=%q want public after Publish", a.Visibility)
	}
}

// TestValidVisibility covers the two-value enum check.
func TestValidVisibility(t *testing.T) {
	cases := map[string]bool{
		"public":  true,
		"private": true,
		"":        false,
		"draft":   false,
	}
	for in, want := range cases {
		if got := domain.ValidVisibility(in); got != want {
			t.Errorf("ValidVisibility(%q)=%v want %v", in, got, want)
		}
	}
}
