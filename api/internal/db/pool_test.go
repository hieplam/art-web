// api/internal/db/pool_test.go
package db_test

import (
	"context"
	"testing"

	"local/art-web/api/internal/db"
	"local/art-web/api/internal/dbtest"
)

func TestNew_PingsOnConstruction(t *testing.T) {
	dsn := startEphemeralPostgres(t)
	pool, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestNew_RejectsBadDSN(t *testing.T) {
	if _, err := db.New(context.Background(), "postgres://nobody@127.0.0.1:1/none"); err == nil {
		t.Fatal("expected error connecting to nonexistent server")
	}
}

func startEphemeralPostgres(t *testing.T) string {
	return dbtest.StartPostgres(t)
}
