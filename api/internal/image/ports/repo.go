// Package ports defines the image slice's outbound interfaces. Implementations
// live under adapters/postgres/, etc. Ports import only the slice's domain
// package — never gorm, chi, zerolog, or any adapter package.
package ports

import (
	"context"

	imagedomain "local/art-web/api/internal/image/domain"
)

// InsertInput is the payload for inserting one image row. Field set + names
// match the persistence struct so the adapter doesn't need to map.
type InsertInput struct {
	ID, ArtworkID, ClientImageID, ContentType, StorageKey, Blurhash, SourceSHA256 string
	Position, Width, Height, ByteSize                                             int
}

// InsertResult tells the caller whether an idempotent insert returned an
// existing row (true) or created a new one (false).
type InsertResult struct {
	ID      string
	Existed bool
}

// ImageRepository is the image slice's persistence port. The concrete adapter
// is image/adapters/postgres.Repo.
type ImageRepository interface {
	Insert(ctx context.Context, in InsertInput) (*InsertResult, error)
	FindByClientImageID(ctx context.Context, artworkID, clientImageID string) (*imagedomain.Image, error)
	ListByArtwork(ctx context.Context, artworkID string) ([]imagedomain.Image, error)
}
