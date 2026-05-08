package domain

import "time"

// Artwork mirrors the persistence shape so the postgres adapter can return it
// directly via a type alias. Tags/Images are populated only when callers
// explicitly preload them — the bare repo Get returns just the row fields.
type Artwork struct {
	ID, UserID, Title, Visibility string
	Description                   *string
	PublishedAt, CreatedAt        *time.Time
	CoverPosition                 int
	Tags                          []Tag   // optional, repo preloads only when asked
	Images                        []Image // optional, repo preloads only when asked
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
