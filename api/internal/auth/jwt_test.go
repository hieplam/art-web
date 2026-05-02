// api/internal/auth/jwt_test.go
package auth_test

import (
	"testing"
	"time"

	"local/art-web/api/internal/auth"
)

func TestJWT_RoundTrip(t *testing.T) {
	j := auth.NewJWT(testKey, func() time.Time { return time.Unix(1000, 0) })
	tok, err := j.Issue("user-id-1", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	sub, err := j.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if sub != "user-id-1" {
		t.Fatalf("sub = %q", sub)
	}
}

func TestJWT_RejectsWrongKey(t *testing.T) {
	a := auth.NewJWT(testKey, time.Now)
	tok, _ := a.Issue("u", time.Hour)
	b := auth.NewJWT([]byte("ffffffffffffffffffffffffffffffff"), time.Now)
	if _, err := b.Verify(tok); err == nil {
		t.Fatal("expected verify failure with wrong key")
	}
}

func TestJWT_RejectsExpired(t *testing.T) {
	clock := time.Unix(1000, 0)
	j := auth.NewJWT(testKey, func() time.Time { return clock })
	tok, _ := j.Issue("u", time.Hour)
	clock = clock.Add(2 * time.Hour)
	if _, err := j.Verify(tok); err == nil {
		t.Fatal("expected expired token to fail verify")
	}
}
