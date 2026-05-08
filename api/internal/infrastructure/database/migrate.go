// api/internal/infrastructure/database/migrate.go
package database

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"local/art-web/api/migrations"
)

func MigrateUp(_ context.Context, dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	target := "pgx5://" + strings.TrimPrefix(dsn, "postgres://")
	m, err := migrate.NewWithSourceInstance("iofs", src, target)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
