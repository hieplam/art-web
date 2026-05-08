// api/internal/infrastructure/database/connection.go
package database

import (
	"errors"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	infraconfig "local/art-web/api/internal/infrastructure/config"
)

// DatabaseConfig aliases config.DatabaseConfig so callers of NewGormDB need
// not import the config package directly (Option A per Task 3 spec).
type DatabaseConfig = infraconfig.DatabaseConfig

// NewGormDB opens a GORM connection. Returns (*gorm.DB, cleanup, error) per
// spec §6.5. AutoMigrate is forbidden (spec §10.2); migrations live in
// api/migrations/*.sql and are run via golang-migrate (see migrate.go).
func NewGormDB(cfg DatabaseConfig) (*gorm.DB, func(), error) {
	if cfg.URL == "" {
		return nil, nil, errors.New("database url required")
	}
	db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	cleanup := func() { _ = sqlDB.Close() }
	return db, cleanup, nil
}

// NewGormDBFromDSN is a convenience constructor for tests that already have
// a DSN string from testcontainers. Production code uses NewGormDB(cfg).
func NewGormDBFromDSN(dsn string) (*gorm.DB, func(), error) {
	return NewGormDB(DatabaseConfig{URL: dsn})
}
