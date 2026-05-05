package user

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestUniqueConstraint_NonPgErrorReturnsEmpty(t *testing.T) {
	got := uniqueConstraint(errors.New("plain error, not a pg error"))
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestUniqueConstraint_PgErrorWrongCodeReturnsEmpty(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42P01", ConstraintName: "irrelevant"}
	got := uniqueConstraint(pgErr)
	if got != "" {
		t.Fatalf("expected empty for non-23505 SQLState, got %q", got)
	}
}

func TestUniqueConstraint_PgErrorReturnsConstraintName(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_slug_key"}
	got := uniqueConstraint(pgErr)
	if got != "users_slug_key" {
		t.Fatalf("expected users_slug_key, got %q", got)
	}
}
