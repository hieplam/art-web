package domain

import "time"

type Artwork struct {
	ID            string
	UserID        string
	Title         string
	Description   *string
	Visibility    Visibility
	CoverPosition int
	CreatedAt     time.Time
	PublishedAt   *time.Time
	Tags          []Tag
	Images        []Image // populated when repo preloads
}

// Image is a slim view; full Image lives in internal/image/domain.
// Duplicated here to avoid cross-slice dependency (spec §4.3 cross-slice rule).
type Image struct {
	ID          string
	Position    int
	StorageKey  string
	ContentType string
	Width       int
	Height      int
	Blurhash    string
}

// Publish flips Visibility to public. Returns ErrAlreadyPublished if already
// public, ErrNoImages if zero images attached. The service catches
// ErrAlreadyPublished and returns nil to preserve the current 204 byte
// (spec §7.3.2).
func (a *Artwork) Publish() error {
	if a.Visibility == VisibilityPublic {
		return ErrAlreadyPublished
	}
	if len(a.Images) == 0 {
		return ErrNoImages
	}
	a.Visibility = VisibilityPublic
	return nil
}
