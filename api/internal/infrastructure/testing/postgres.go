// api/internal/infrastructure/testing/postgres.go
package testing

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/gorm"

	"local/art-web/api/migrations"
)

var (
	once      sync.Once
	sharedDSN string
	sharedErr error
)

func StartPostgres(t testing.TB) string {
	t.Helper()
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		container, err := tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("artweb"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
			tcpostgres.WithSQLDriver("pgx"),
		)
		if err != nil {
			sharedErr = err
			return
		}
		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedErr = err
			return
		}
		src, err := iofs.New(migrations.FS, ".")
		if err != nil {
			sharedErr = err
			return
		}
		m, err := migrate.NewWithSourceInstance("iofs", src, "pgx5://"+strings.TrimPrefix(dsn, "postgres://"))
		if err != nil {
			sharedErr = err
			return
		}
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			sharedErr = err
			return
		}
		sharedDSN = dsn
	})
	if sharedErr != nil {
		t.Fatalf("postgres harness: %v", sharedErr)
	}
	return sharedDSN
}

// TruncateAll wipes every business table in the shared test database. The
// exec callback must run the SQL against the active DB handle (the GORM
// adapter passes db.WithContext(ctx).Exec(sql).Error).
func TruncateAll(t testing.TB, exec func(ctx context.Context, sql string, args ...any) error) {
	t.Helper()
	if err := exec(context.Background(), `
		TRUNCATE TABLE artwork_tags, tags, artwork_images, artworks, users RESTART IDENTITY CASCADE;
	`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// TruncateAllGorm is a convenience wrapper for callers that already hold a
// *gorm.DB; it adapts the call into the legacy exec-callback shape.
func TruncateAllGorm(t testing.TB, db *gorm.DB) {
	t.Helper()
	TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		return db.WithContext(ctx).Exec(sql).Error
	})
}
