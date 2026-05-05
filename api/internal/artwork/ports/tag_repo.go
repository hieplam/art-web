package ports

import "context"

// TagRepository is the artwork slice's tag-persistence port. Implemented by
// artwork/adapters/postgres.TagsRepo.
type TagRepository interface {
	GetTags(ctx context.Context, artworkID string) ([]string, error)
	SetTags(ctx context.Context, artworkID string, tags []string) error
}
