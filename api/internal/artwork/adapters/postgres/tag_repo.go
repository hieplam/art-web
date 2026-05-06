// api/internal/artwork/adapters/postgres/tag_repo.go
package postgres

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"local/art-web/api/internal/infrastructure/database"
)

// TagsRepo is the GORM-backed adapter for the tags / artwork_tags tables.
type TagsRepo struct{ db *gorm.DB }

// NewTagsRepo constructs a tag repository against the root *gorm.DB. Like
// the artwork repo, it resolves database.DB(ctx, r.db) inside each method so
// callers may compose it under an outer Transactor (spec §7.6.1).
func NewTagsRepo(db *gorm.DB) *TagsRepo { return &TagsRepo{db: db} }

func (t *TagsRepo) SetTags(ctx context.Context, artworkID string, raw []string) error {
	return database.DB(ctx, t.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("artwork_id = ?", artworkID).
			Delete(&artworkTagModel{}).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, r := range raw {
			n := strings.ToLower(strings.TrimSpace(r))
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			// ON CONFLICT (name) DO UPDATE … RETURNING id mirrors the legacy
			// upsert. GORM doesn't expose that directly, so fall back to raw
			// SQL — the trailing UPDATE is a no-op that lets RETURNING fire
			// on existing rows the same way the pgx version did.
			var id string
			if err := tx.Raw(`
				INSERT INTO tags (name) VALUES (?)
				ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
				RETURNING id`, n).Scan(&id).Error; err != nil {
				return err
			}
			if err := tx.Create(&artworkTagModel{ArtworkID: artworkID, TagID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (t *TagsRepo) GetTags(ctx context.Context, artworkID string) ([]string, error) {
	var names []string
	err := database.DB(ctx, t.db).WithContext(ctx).
		Table("tags AS t").
		Select("t.name").
		Joins("JOIN artwork_tags atag ON atag.tag_id = t.id").
		Where("atag.artwork_id = ?", artworkID).
		Order("t.name").
		Scan(&names).Error
	if err != nil {
		return nil, err
	}
	return names, nil
}
