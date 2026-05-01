// api/internal/artwork/tags.go
package artwork

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TagsRepo struct{ pool *pgxpool.Pool }

func NewTagsRepo(p *pgxpool.Pool) *TagsRepo { return &TagsRepo{pool: p} }

func (t *TagsRepo) SetTags(ctx context.Context, artworkID string, raw []string) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM artwork_tags WHERE artwork_id = $1`, artworkID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range raw {
		n := strings.ToLower(strings.TrimSpace(r))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO tags (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, n).Scan(&id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO artwork_tags (artwork_id, tag_id) VALUES ($1,$2)`,
			artworkID, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (t *TagsRepo) GetTags(ctx context.Context, artworkID string) ([]string, error) {
	rows, err := t.pool.Query(ctx, `
		SELECT t.name FROM tags t
		JOIN artwork_tags atag ON atag.tag_id = t.id
		WHERE atag.artwork_id = $1
		ORDER BY t.name`, artworkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		out = append(out, n)
	}
	return out, nil
}
