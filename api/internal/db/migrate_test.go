// api/internal/db/migrate_test.go
package db_test

import (
	"context"
	"testing"

	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
)

func TestMigrate_IsIdempotent(t *testing.T) {
	dsn := dbtest.StartPostgres(t)
	if err := db.MigrateUp(context.Background(), dsn); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}
	if err := db.MigrateUp(context.Background(), dsn); err != nil {
		t.Fatalf("second MigrateUp: %v", err)
	}
}
