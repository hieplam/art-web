// api/internal/dbtest/postgres.go
package dbtest

import (
	"context"
	"testing"
)

// StartPostgres is a placeholder — full implementation in Task 4.
func StartPostgres(t testing.TB) string {
	t.Helper()
	t.Fatal("dbtest.StartPostgres not yet implemented (Task 4)")
	return ""
}

// TruncateAll truncates all test tables.
func TruncateAll(t testing.TB, exec func(ctx context.Context, sql string, args ...any) error) {
	t.Helper()
	if err := exec(context.Background(), `
		TRUNCATE TABLE artwork_tags, tags, artwork_images, artworks, users RESTART IDENTITY CASCADE;
	`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
