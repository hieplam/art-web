package artwork

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"local/art-web/api/internal/storage"
)

type VisibilityService struct {
	repo  *Repo
	store storage.Storage
}

func NewVisibilityService(r *Repo, s storage.Storage) *VisibilityService {
	return &VisibilityService{repo: r, store: s}
}

type flipMove struct{ id, src, dst string }

func (v *VisibilityService) Flip(ctx context.Context, artworkID, target string) error {
	if target != "public" && target != "private" {
		return errBadTarget
	}
	a, err := v.repo.Get(ctx, artworkID)
	if err != nil {
		return err
	}
	if a.Visibility == target {
		return nil
	}

	rows, err := v.repo.Pool().Query(ctx, `SELECT id, storage_key FROM artwork_images WHERE artwork_id=$1`, artworkID)
	if err != nil {
		return err
	}
	var moves []flipMove
	for rows.Next() {
		var id, src string
		if err := rows.Scan(&id, &src); err != nil {
			rows.Close()
			return err
		}
		if !strings.HasPrefix(src, a.Visibility+"/") {
			rows.Close()
			return fmt.Errorf("storage_key %q does not match artwork visibility %q", src, a.Visibility)
		}
		dst := target + strings.TrimPrefix(src, a.Visibility)
		moves = append(moves, flipMove{id, src, dst})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	completed := moves[:0:0]
	for _, m := range moves {
		if err := v.store.Move(ctx, m.src, m.dst); err != nil {
			rollbackMoves(ctx, v.store, completed)
			return fmt.Errorf("move %s→%s: %w", m.src, m.dst, err)
		}
		completed = append(completed, m)
	}

	tx, err := v.repo.Pool().Begin(ctx)
	if err != nil {
		rollbackMoves(ctx, v.store, completed)
		return err
	}
	defer tx.Rollback(ctx)
	for _, m := range moves {
		if _, err := tx.Exec(ctx,
			`UPDATE artwork_images SET storage_key=$2 WHERE id=$1`, m.id, m.dst); err != nil {
			rollbackMoves(ctx, v.store, completed)
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE artworks SET
		  visibility=$2,
		  published_at = CASE WHEN $2='public' THEN COALESCE(published_at, now()) ELSE published_at END
		WHERE id=$1`, artworkID, target); err != nil {
		rollbackMoves(ctx, v.store, completed)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		rollbackMoves(ctx, v.store, completed)
		return err
	}
	return nil
}

func rollbackMoves(ctx context.Context, store storage.Storage, completed []flipMove) {
	for i := len(completed) - 1; i >= 0; i-- {
		m := completed[i]
		if err := store.Move(ctx, m.dst, m.src); err != nil {
			RollbackLog(fmt.Errorf("rollback move %s→%s: %w", m.dst, m.src, err))
		}
	}
}

var RollbackLog = func(err error) {}

var errBadTarget = errors.New("target must be 'public' or 'private'")
