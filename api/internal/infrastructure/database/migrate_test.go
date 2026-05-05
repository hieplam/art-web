// api/internal/infrastructure/database/migrate_test.go
package database_test

import (
	"context"
	"testing"

	"local/art-web/api/internal/infrastructure/database"
	infratest "local/art-web/api/internal/infrastructure/testing"
)

func TestMigrate_IsIdempotent(t *testing.T) {
	dsn := infratest.StartPostgres(t)
	if err := database.MigrateUp(context.Background(), dsn); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}
	if err := database.MigrateUp(context.Background(), dsn); err != nil {
		t.Fatalf("second MigrateUp: %v", err)
	}
}
