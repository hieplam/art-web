// api/internal/artwork/adapters/postgres/repo.go
package postgres

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	artworkdomain "local/art-web/api/internal/artwork/domain"
	"local/art-web/api/internal/artwork/ports"
	"local/art-web/api/internal/infrastructure/database"
)

// Artwork is the persistence-shape entity returned by Repo. It is an alias for
// the domain entity so handlers can read fields directly without a per-call
// mapping shim at the adapter boundary.
type Artwork = artworkdomain.Artwork

// Repo is the artwork slice's GORM-backed persistence adapter.
type Repo struct{ db *gorm.DB }

// NewRepo constructs a repo with the root *gorm.DB. database.DB(ctx, r.db)
// resolves to the active handle (root or transactional) per spec §7.6.1.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB returns the underlying root *gorm.DB. Tests use this to seed fixtures
// directly; production code should not reach into it.
func (r *Repo) DB() *gorm.DB { return r.db }

// FeedCursor and FeedPage are aliased to the ports types so callers may use
// either name interchangeably; ports owns the canonical definitions.
type (
	FeedCursor = ports.FeedCursor
	FeedPage   = ports.FeedPage
	FlipMove   = ports.FlipMove
)

func (r *Repo) Create(ctx context.Context, userID, title, description, visibility string) (string, error) {
	db := database.DB(ctx, r.db).WithContext(ctx)
	var descPtr *string
	if description != "" {
		descPtr = &description
	}
	m := artworkModel{
		UserID:      userID,
		Title:       title,
		Description: descPtr,
		Visibility:  visibility,
	}
	// Mirror the SQL CASE: published_at = now() iff visibility='public'.
	// We rely on Postgres to apply DEFAULT now() for created_at and
	// gen_random_uuid() for id (both declared in the migration; tags include
	// the matching `default:` clauses).
	if visibility == "public" {
		// Use expression so the timestamp comes from the DB clock, not Go.
		err := db.Raw(`
			INSERT INTO artworks (user_id, title, description, visibility, published_at)
			VALUES (?, ?, NULLIF(?, ''), ?, now())
			RETURNING id`,
			userID, title, description, visibility).Scan(&m.ID).Error
		if err != nil {
			return "", err
		}
		return m.ID, nil
	}
	if err := db.Create(&m).Error; err != nil {
		return "", err
	}
	return m.ID, nil
}

func (r *Repo) Get(ctx context.Context, id string) (*Artwork, error) {
	var m artworkModel
	err := database.DB(ctx, r.db).WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, artworkdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toDomainArtwork(&m), nil
}

func (r *Repo) PatchTitle(ctx context.Context, id, title string) error {
	return database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).Where("id = ?", id).
		Update("title", title).Error
}

func (r *Repo) PatchDescription(ctx context.Context, id, description string) error {
	var v any
	if description == "" {
		v = nil
	} else {
		v = description
	}
	return database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).Where("id = ?", id).
		Update("description", v).Error
}

func (r *Repo) PatchVisibility(ctx context.Context, id, vis string) error {
	if vis == "public" {
		return database.DB(ctx, r.db).WithContext(ctx).Exec(`
			UPDATE artworks
			SET visibility = ?,
			    published_at = COALESCE(published_at, now())
			WHERE id = ?`, vis, id).Error
	}
	return database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).Where("id = ?", id).
		Update("visibility", vis).Error
}

func (r *Repo) SetCoverPosition(ctx context.Context, id string, position int) error {
	return database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).Where("id = ?", id).
		Update("cover_position", position).Error
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	return database.DB(ctx, r.db).WithContext(ctx).
		Where("id = ?", id).Delete(&artworkModel{}).Error
}

// PublicFeed returns the next page of public artworks ordered by published_at
// DESC, id DESC. The cursor's (stamp, id) tuple — when non-zero — strictly
// excludes rows newer than or equal to it.
func (r *Repo) PublicFeed(ctx context.Context, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	q := database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).
		Where("visibility = ? AND published_at IS NOT NULL", "public")
	if !c.IsZero() {
		q = q.Where("(published_at, id) < (?, ?)", c.Stamp, c.ID)
	}
	var rows []artworkModel
	if err := q.Order("published_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToFeedPage(rows, limit, false), nil
}

func (r *Repo) ListByUser(ctx context.Context, userID string, viewerIsOwner bool, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	q := database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).
		Where("user_id = ?", userID)
	if !viewerIsOwner {
		q = q.Where("visibility = ? AND published_at IS NOT NULL", "public")
	}
	if !c.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", c.Stamp, c.ID)
	}
	var rows []artworkModel
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToFeedPage(rows, limit, true), nil
}

func (r *Repo) ListByTag(ctx context.Context, tag string, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	q := database.DB(ctx, r.db).WithContext(ctx).
		Table("artworks AS a").
		Select("a.*").
		Joins("JOIN artwork_tags atag ON atag.artwork_id = a.id").
		Joins("JOIN tags t ON t.id = atag.tag_id").
		Where("t.name = lower(?)", tag).
		Where("a.visibility = ? AND a.published_at IS NOT NULL", "public")
	if !c.IsZero() {
		q = q.Where("(a.published_at, a.id) < (?, ?)", c.Stamp, c.ID)
	}
	var rows []artworkModel
	if err := q.Order("a.published_at DESC, a.id DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToFeedPage(rows, limit, false), nil
}

// rowsToFeedPage converts a slice of GORM rows into a domain FeedPage,
// trimming to limit and synthesizing the next cursor when extra rows were
// fetched. useCreatedAt selects which column drives the cursor stamp; feeds
// that filter on published_at use that column instead.
func rowsToFeedPage(rows []artworkModel, limit int, useCreatedAt bool) *FeedPage {
	out := &FeedPage{}
	for _, m := range rows {
		out.Items = append(out.Items, *toDomainArtwork(&m))
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		var stamp = *last.CreatedAt
		if !useCreatedAt {
			// PublicFeed and ListByTag both filter on published_at IS NOT NULL,
			// so the deref is always safe in this branch.
			stamp = *last.PublishedAt
		}
		out.NextCursor = &FeedCursor{Stamp: stamp, ID: last.ID}
	}
	return out
}

// PendingFlipMoves enumerates the storage-key transitions implied by flipping
// an artwork from fromVis to toVis. Each row's existing storage_key must be
// prefixed with fromVis+"/"; the destination key swaps that prefix to toVis.
// Returns an error if any row's key fails the prefix invariant — that's a
// data-shape inconsistency the caller should surface.
func (r *Repo) PendingFlipMoves(ctx context.Context, artworkID, fromVis, toVis string) ([]FlipMove, error) {
	var rows []artworkImageModel
	err := database.DB(ctx, r.db).WithContext(ctx).
		Where("artwork_id = ?", artworkID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	var moves []FlipMove
	for _, im := range rows {
		if !strings.HasPrefix(im.StorageKey, fromVis+"/") {
			return nil, &flipPrefixError{src: im.StorageKey, fromVis: fromVis}
		}
		// We've already verified the key starts with fromVis+"/", so the
		// remainder is everything from len(fromVis) onward (which begins with
		// the slash). Splicing on the slice index is clearer than the older
		// TrimPrefix(fromVis) form, which depended on TrimPrefix retaining
		// the slash byte after a fromVis-only match.
		dst := toVis + im.StorageKey[len(fromVis):]
		moves = append(moves, FlipMove{ID: im.ID, Src: im.StorageKey, Dst: dst})
	}
	return moves, nil
}

// FinalizeFlip updates artwork_images.storage_key to the new dst values for
// the completed moves and flips artworks.visibility to target — both inside a
// single transaction so a partial failure cannot leak files into the wrong
// visibility prefix while the artwork row still claims the old visibility.
func (r *Repo) FinalizeFlip(ctx context.Context, artworkID, target string, completed []FlipMove) error {
	return database.DB(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, m := range completed {
			if err := tx.Model(&artworkImageModel{}).
				Where("id = ?", m.ID).
				Update("storage_key", m.Dst).Error; err != nil {
				return err
			}
		}
		if target == "public" {
			return tx.Exec(`
				UPDATE artworks SET
				  visibility = ?,
				  published_at = COALESCE(published_at, now())
				WHERE id = ?`, target, artworkID).Error
		}
		return tx.Model(&artworkModel{}).Where("id = ?", artworkID).
			Update("visibility", target).Error
	})
}

// flipPrefixError lets the visibility service surface a precise error when an
// image's storage_key isn't prefixed by the artwork's current visibility — a
// data invariant that would silently corrupt the flip if ignored.
type flipPrefixError struct {
	src, fromVis string
}

func (e *flipPrefixError) Error() string {
	return "storage_key " + e.src + " does not match artwork visibility " + e.fromVis
}
