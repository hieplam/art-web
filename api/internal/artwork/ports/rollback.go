package ports

import "context"

// RollbackReporter is invoked when a compensating storage-rollback move fails
// after a primary visibility-flip operation has already failed (a leak).
// Per spec §7.7, fired only when the *undo* itself fails — not on the primary
// failure. The default implementation lives in artwork/adapters/log.
type RollbackReporter interface {
	Report(ctx context.Context, imageID string, err error)
}
