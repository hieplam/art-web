// Package ports defines the artwork slice's outbound interfaces. Implementations
// live under adapters/postgres/, adapters/log/, etc. Ports import only the
// slice's domain package — never gorm, chi, zerolog, or any adapter package
// (spec §4.3 banned-import list).
package ports

import (
	"context"
	"time"

	artworkdomain "local/art-web/api/internal/artwork/domain"
)

// FeedCursor carries the full-precision timestamp + id of the last row on the
// previous feed page. Stamp.IsZero() means "first page".
type FeedCursor struct {
	Stamp time.Time
	ID    string
}

// IsZero reports whether the cursor points at the first page.
func (c FeedCursor) IsZero() bool { return c.Stamp.IsZero() && c.ID == "" }

// FeedPage is one page of feed results plus an optional cursor for the next.
type FeedPage struct {
	Items      []artworkdomain.Artwork
	NextCursor *FeedCursor
}

// FlipMove is one image's storage-key transition during a visibility flip.
type FlipMove struct {
	ID  string // artwork_image row ID
	Src string // current storage key
	Dst string // target storage key
}

// ArtworkRepository is the artwork slice's primary persistence port. The
// concrete adapter lives in artwork/adapters/postgres.
type ArtworkRepository interface {
	Get(ctx context.Context, id string) (*artworkdomain.Artwork, error)
	Create(ctx context.Context, userID, title, description, visibility string) (string, error)
	PatchTitle(ctx context.Context, id, title string) error
	PatchDescription(ctx context.Context, id, description string) error
	SetCoverPosition(ctx context.Context, id string, pos int) error
	Delete(ctx context.Context, id string) error

	PublicFeed(ctx context.Context, cursor FeedCursor, limit int) (*FeedPage, error)
	ListByUser(ctx context.Context, userID string, viewerIsOwner bool, cursor FeedCursor, limit int) (*FeedPage, error)
	ListByTag(ctx context.Context, tag string, cursor FeedCursor, limit int) (*FeedPage, error)

	// Visibility-flip support (spec §7.6.1). PendingFlipMoves enumerates the
	// (id, src, dst) tuples for an artwork's images. FinalizeFlip updates the
	// image storage_key column to dst for each completed move and flips the
	// artwork's visibility — both inside one DB transaction.
	PendingFlipMoves(ctx context.Context, artworkID, fromVis, toVis string) ([]FlipMove, error)
	FinalizeFlip(ctx context.Context, artworkID, target string, completed []FlipMove) error
}
