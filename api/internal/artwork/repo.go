// api/internal/artwork/repo.go
package artwork

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Artwork struct {
	ID, UserID, Title, Visibility string
	Description                   *string
	PublishedAt, CreatedAt        *time.Time
	CoverPosition                 int
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(p *pgxpool.Pool) *Repo { return &Repo{pool: p} }

func (r *Repo) Pool() *pgxpool.Pool { return r.pool }

func (r *Repo) Create(ctx context.Context, userID, title, description, visibility string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO artworks (user_id, title, description, visibility, published_at)
		VALUES ($1, $2, NULLIF($3,''), $4, CASE WHEN $4='public' THEN now() ELSE NULL END)
		RETURNING id`,
		userID, title, description, visibility).Scan(&id)
	return id, err
}

func (r *Repo) Get(ctx context.Context, id string) (*Artwork, error) {
	var a Artwork
	var desc *string
	var pub *time.Time
	var cre time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, title, description, visibility, published_at, created_at, cover_position
		FROM artworks WHERE id = $1`, id).
		Scan(&a.ID, &a.UserID, &a.Title, &desc, &a.Visibility, &pub, &cre, &a.CoverPosition)
	if err != nil {
		return nil, err
	}
	a.Description = desc
	a.PublishedAt = pub
	a.CreatedAt = &cre
	return &a, nil
}

func (r *Repo) PatchTitle(ctx context.Context, id, title string) error {
	_, err := r.pool.Exec(ctx, `UPDATE artworks SET title=$2 WHERE id=$1`, id, title)
	return err
}

func (r *Repo) PatchDescription(ctx context.Context, id, description string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE artworks SET description=NULLIF($2,'') WHERE id=$1`, id, description)
	return err
}

func (r *Repo) PatchVisibility(ctx context.Context, id, vis string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE artworks
		SET visibility = $2,
		    published_at = CASE WHEN $2='public' THEN COALESCE(published_at, now()) ELSE published_at END
		WHERE id = $1`, id, vis)
	return err
}

func (r *Repo) SetCoverPosition(ctx context.Context, id string, position int) error {
	_, err := r.pool.Exec(ctx, `UPDATE artworks SET cover_position=$2 WHERE id=$1`, id, position)
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM artworks WHERE id=$1`, id)
	return err
}

// FeedCursor carries the full-precision timestamp of the last row on the
// previous page. Stamp.IsZero() means "first page".
type FeedCursor struct {
	Stamp time.Time
	ID    string
}

func (c FeedCursor) IsZero() bool { return c.Stamp.IsZero() && c.ID == "" }

type FeedPage struct {
	Items      []Artwork
	NextCursor *FeedCursor
}

func (r *Repo) PublicFeed(ctx context.Context, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, description, visibility, published_at, created_at, cover_position
		FROM artworks
		WHERE visibility='public' AND published_at IS NOT NULL
		  AND ($1::boolean OR (published_at, id) < ($2::timestamptz, $3::uuid))
		ORDER BY published_at DESC, id DESC
		LIMIT $4`,
		c.IsZero(), nullableStamp(c), nullableID(c), limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedRows(rows, limit, false)
}

func nullableStamp(c FeedCursor) any {
	if c.IsZero() {
		return nil
	}
	return c.Stamp
}
func nullableID(c FeedCursor) any {
	if c.IsZero() {
		return nil
	}
	return c.ID
}

func (r *Repo) ListByUser(ctx context.Context, userID string, viewerIsOwner bool, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	q := `
		SELECT id, user_id, title, description, visibility, published_at, created_at, cover_position
		FROM artworks
		WHERE user_id = $1 ` +
		map[bool]string{
			true:  ``,
			false: ` AND visibility='public' AND published_at IS NOT NULL`,
		}[viewerIsOwner] + `
		  AND ($2::boolean OR (created_at, id) < ($3::timestamptz, $4::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $5`
	rows, err := r.pool.Query(ctx, q, userID, c.IsZero(), nullableStamp(c), nullableID(c), limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedRows(rows, limit, true)
}

func scanFeedRows(rows pgx.Rows, limit int, useCreatedAt bool) (*FeedPage, error) {
	out := &FeedPage{}
	for rows.Next() {
		var a Artwork
		var desc *string
		var pub *time.Time
		var cre time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.Title, &desc, &a.Visibility, &pub, &cre, &a.CoverPosition); err != nil {
			return nil, err
		}
		a.PublishedAt = pub
		a.CreatedAt = &cre
		a.Description = desc
		out.Items = append(out.Items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		var stamp time.Time
		if useCreatedAt {
			stamp = *last.CreatedAt
		} else if last.PublishedAt != nil {
			stamp = *last.PublishedAt
		} else {
			stamp = *last.CreatedAt
		}
		out.NextCursor = &FeedCursor{Stamp: stamp, ID: last.ID}
	}
	return out, nil
}

func (r *Repo) ListByTag(ctx context.Context, tag string, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.user_id, a.title, a.description, a.visibility, a.published_at, a.created_at, a.cover_position
		FROM artworks a
		JOIN artwork_tags atag ON atag.artwork_id = a.id
		JOIN tags t ON t.id = atag.tag_id
		WHERE t.name = lower($1)
		  AND a.visibility='public' AND a.published_at IS NOT NULL
		  AND ($2::boolean OR (a.published_at, a.id) < ($3::timestamptz, $4::uuid))
		ORDER BY a.published_at DESC, a.id DESC
		LIMIT $5`, tag, c.IsZero(), nullableStamp(c), nullableID(c), limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedRows(rows, limit, false)
}
