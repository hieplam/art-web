package service

import (
	"context"
	"errors"
	"fmt"

	"local/art-web/api/internal/artwork/ports"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
)

// VisibilityService orchestrates the storage-first, DB-within-tx, compensate-
// on-failure visibility flip described in spec §7.6.1. The repo, storage, and
// rollback reporter are injected so unit tests can stand in fakes for any of
// them.
type VisibilityService struct {
	repo     ports.ArtworkRepository
	store    infrastorage.Storage
	rollback ports.RollbackReporter
}

// NewVisibilityService accepts the artwork repo and rollback reporter as ports
// so any conforming implementation works. The legacy two-arg constructor lives
// alongside as NewVisibilityServiceLegacy for tests that still call it that
// way; new wiring should use this one.
func NewVisibilityService(repo ports.ArtworkRepository, store infrastorage.Storage, rollback ports.RollbackReporter) *VisibilityService {
	if rollback == nil {
		rollback = noopRollbackReporter{}
	}
	return &VisibilityService{repo: repo, store: store, rollback: rollback}
}

// Flip applies the storage-first visibility transition. Steps mirror the
// existing pgx-based implementation:
//  1. Read artwork; early-return on same-visibility no-op.
//  2. Enumerate pending flip moves; move each storage object.
//     Failure here triggers reverse moves; no DB writes happened yet.
//  3. Call FinalizeFlip which updates image keys + artwork.visibility in one tx.
//     Failure here triggers reverse moves to keep storage in sync with the
//     DB row that's still claiming the old visibility.
//
// rollback.Report fires only when a *reverse* (undo) move itself fails — the
// leak window the original RollbackLog also instrumented.
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

	moves, err := v.repo.PendingFlipMoves(ctx, artworkID, a.Visibility, target)
	if err != nil {
		return err
	}

	completed := moves[:0:0]
	for _, m := range moves {
		if err := v.store.Move(ctx, m.Src, m.Dst); err != nil {
			v.rollbackMoves(ctx, completed)
			return fmt.Errorf("move %s→%s: %w", m.Src, m.Dst, err)
		}
		completed = append(completed, m)
	}

	if err := v.repo.FinalizeFlip(ctx, artworkID, target, completed); err != nil {
		v.rollbackMoves(ctx, completed)
		return err
	}
	return nil
}

func (v *VisibilityService) rollbackMoves(ctx context.Context, completed []ports.FlipMove) {
	for i := len(completed) - 1; i >= 0; i-- {
		m := completed[i]
		if err := v.store.Move(ctx, m.Dst, m.Src); err != nil {
			v.rollback.Report(ctx, m.ID, fmt.Errorf("rollback move %s→%s: %w", m.Dst, m.Src, err))
		}
	}
}

// noopRollbackReporter is the safety default when the constructor is called
// with a nil reporter — the rollback path won't panic mid-flight.
type noopRollbackReporter struct{}

func (noopRollbackReporter) Report(_ context.Context, _ string, _ error) {}

var errBadTarget = errors.New("target must be 'public' or 'private'")
