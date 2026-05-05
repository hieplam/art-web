package image

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
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "artwork_images_artwork_id_position_key"}
	got := uniqueConstraint(pgErr)
	if got != "artwork_images_artwork_id_position_key" {
		t.Fatalf("expected artwork_images_artwork_id_position_key, got %q", got)
	}
}

func TestInsert_RejectsEmptySourceSHA256(t *testing.T) {
	r := &Repo{pool: nil} // pool is never reached; the guard returns early.
	_, err := r.Insert(nil, InsertInput{SourceSHA256: ""})
	if err == nil || err.Error() != "SourceSHA256 is required" {
		t.Fatalf("expected SourceSHA256 required error, got %v", err)
	}
}
