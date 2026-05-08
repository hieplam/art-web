// api/internal/infrastructure/database/transactor.go
package database

import (
	"context"

	"gorm.io/gorm"
)

type ctxKey struct{}

// Transactor wraps a *gorm.DB and exposes WithinTx. Repo methods call
// DB(ctx, fallback) to get the active handle.
type Transactor struct{ db *gorm.DB }

func NewTransactor(db *gorm.DB) *Transactor { return &Transactor{db: db} }

// WithinTx runs fn inside a single transaction. The tx-bound *gorm.DB is
// stashed in ctx; repos retrieve it via DB(ctx, r.db).
func (t *Transactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxKey{}, tx))
	})
}

// DB returns the active gorm handle: the transactional one if a tx is in flight,
// otherwise the fallback (request-scoped or root *gorm.DB).
func DB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok {
		return tx
	}
	return fallback
}
