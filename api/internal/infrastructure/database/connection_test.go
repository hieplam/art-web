// api/internal/infrastructure/database/connection_test.go
package database_test

import (
	"context"
	"testing"

	"local/art-web/api/internal/infrastructure/database"
	infratest "local/art-web/api/internal/infrastructure/testing"
)

func TestNew_PingsOnConstruction(t *testing.T) {
	dsn := startEphemeralPostgres(t)
	pool, err := database.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestNew_RejectsBadDSN(t *testing.T) {
	if _, err := database.New(context.Background(), "postgres://nobody@127.0.0.1:1/none"); err == nil {
		t.Fatal("expected error connecting to nonexistent server")
	}
}

func TestNew_BadDSN_ReturnsError(t *testing.T) {
	_, err := database.New(context.Background(), "this-is-not-a-valid-dsn")
	if err == nil {
		t.Fatal("expected error for malformed DSN")
	}
}

func startEphemeralPostgres(t *testing.T) string {
	return infratest.StartPostgres(t)
}
