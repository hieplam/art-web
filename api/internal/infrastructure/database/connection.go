// api/internal/infrastructure/database/connection.go
package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	infraconfig "local/art-web/api/internal/infrastructure/config"
)

type Pool = pgxpool.Pool

// DatabaseConfig aliases config.DatabaseConfig so callers of NewGormDB need
// not import the config package directly (Option A per Task 3 spec).
type DatabaseConfig = infraconfig.DatabaseConfig

func New(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// NewGormDB opens a GORM connection. Unused until Task 8 swaps consumers.
// Returns (*gorm.DB, cleanup, error) per spec §6.5.
func NewGormDB(cfg DatabaseConfig) (*gorm.DB, func(), error) {
	if cfg.URL == "" {
		return nil, nil, errors.New("database url required")
	}
	db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{
		// Don't auto-migrate; migrations live in api/migrations/*.sql
		// per spec §10.2 (forbidden).
	})
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return db, cleanup, nil
}
