package postgres

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

func TestToDomainUser_NilAvatarBecomesEmpty(t *testing.T) {
	m := &userModel{ID: "u1", Slug: "alice", DisplayName: "Alice", Email: "a@b", AvatarURL: nil}
	u := toDomainUser(m)
	if u.AvatarURL != "" {
		t.Fatalf("nil AvatarURL should map to empty string, got %q", u.AvatarURL)
	}
	if u.ID != "u1" || u.Slug != "alice" || u.DisplayName != "Alice" || u.Email != "a@b" {
		t.Fatalf("unexpected mapping: %+v", u)
	}
}

func TestToDomainUser_PtrAvatarPropagates(t *testing.T) {
	v := "https://cdn/example.png"
	m := &userModel{ID: "u2", Slug: "bob", DisplayName: "Bob", Email: "b@b", AvatarURL: &v}
	u := toDomainUser(m)
	if u.AvatarURL != v {
		t.Fatalf("got %q want %q", u.AvatarURL, v)
	}
}
