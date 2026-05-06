// api/internal/infrastructure/database/connection_test.go
package database_test

import (
	"testing"

	"local/art-web/api/internal/infrastructure/database"
	infratest "local/art-web/api/internal/infrastructure/testing"
)

func TestNewGormDB_PingsOnConstruction(t *testing.T) {
	dsn := startEphemeralPostgres(t)
	db, cleanup, err := database.NewGormDBFromDSN(dsn)
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	defer cleanup()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestNewGormDB_RejectsBadDSN(t *testing.T) {
	if _, _, err := database.NewGormDBFromDSN("postgres://nobody@127.0.0.1:1/none"); err == nil {
		t.Fatal("expected error connecting to nonexistent server")
	}
}

func TestNewGormDB_BadDSN_ReturnsError(t *testing.T) {
	_, _, err := database.NewGormDBFromDSN("this-is-not-a-valid-dsn")
	if err == nil {
		t.Fatal("expected error for malformed DSN")
	}
}

func TestNewGormDB_EmptyURL_ReturnsError(t *testing.T) {
	_, _, err := database.NewGormDBFromDSN("")
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func startEphemeralPostgres(t *testing.T) string {
	return infratest.StartPostgres(t)
}
