// api/internal/auth/service/jwt_test.go
package service_test

import (
	"testing"
	"time"

	authservice "local/art-web/api/internal/auth/service"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestJWT_RoundTrip(t *testing.T) {
	j := authservice.NewJWT(testKey, func() time.Time { return time.Unix(1000, 0) })
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
	a := authservice.NewJWT(testKey, time.Now)
	tok, _ := a.Issue("u", time.Hour)
	b := authservice.NewJWT([]byte("ffffffffffffffffffffffffffffffff"), time.Now)
	if _, err := b.Verify(tok); err == nil {
		t.Fatal("expected verify failure with wrong key")
	}
}

func TestJWT_RejectsExpired(t *testing.T) {
	clock := time.Unix(1000, 0)
	j := authservice.NewJWT(testKey, func() time.Time { return clock })
	tok, _ := j.Issue("u", time.Hour)
	clock = clock.Add(2 * time.Hour)
	if _, err := j.Verify(tok); err == nil {
		t.Fatal("expected expired token to fail verify")
	}
}
