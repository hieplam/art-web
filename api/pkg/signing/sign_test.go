// api/pkg/signing/sign_test.go
package signing_test

import (
	"strings"
	"testing"

	"local/art-web/api/pkg/signing"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestSign_DeterministicForFixedExp(t *testing.T) {
	a, _ := signing.SignCanonical(testKey, "/private/abc/def.jpg", 1700000000)
	b, _ := signing.SignCanonical(testKey, "/private/abc/def.jpg", 1700000000)
	if a != b || len(a) != 64 {
		t.Fatalf("non-deterministic or wrong length: %s vs %s", a, b)
	}
}

func TestSign_DistinctForDifferentInputs(t *testing.T) {
	cases := []struct {
		p   string
		exp int64
	}{
		{"/private/a/b.jpg", 100}, {"/private/a/b.jpg", 101},
		{"/private/a/c.jpg", 100}, {"/public/a/b.jpg", 100},
	}
	seen := map[string]string{}
	for _, c := range cases {
		s, _ := signing.SignCanonical(testKey, c.p, c.exp)
		key := c.p + "|" + s
		if prev, ok := seen[s]; ok {
			t.Fatalf("collision: %s == %s for both %s and %s", s, prev, c.p, key)
		}
		seen[s] = key
	}
}

func TestCanonicalize_RejectsBadPaths(t *testing.T) {
	bad := []string{"/private//x.jpg", "/private/../etc.jpg", "/private/./x.jpg", "/private/x.jpg/"}
	for _, p := range bad {
		if err := signing.ValidateCanonicalPath(p); err == nil {
			t.Errorf("expected reject for %q", p)
		}
	}
}

func TestCanonicalize_RejectsNonAllowedChars(t *testing.T) {
	bad := []string{"/private/abc def.jpg", "/private/abc%20.jpg", "/private/abc?q=1"}
	for _, p := range bad {
		if err := signing.ValidateCanonicalPath(p); err == nil {
			t.Errorf("expected reject for %q", p)
		}
	}
}

func TestCanonicalize_AcceptsSpecKey(t *testing.T) {
	good := "/private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0a1b2c3d-4e5f-6789-abcd-ef0123456789.jpg"
	if err := signing.ValidateCanonicalPath(good); err != nil {
		t.Errorf("rejected good path: %v", err)
	}
}

func TestSign_FixedVector(t *testing.T) {
	const want = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654"
	got, err := signing.SignCanonical(testKey, "/private/aaa/bbb.jpg", 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("signature drifted:\n  got:  %s\n  want: %s", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("expected 64-char hex, got %d chars", len(got))
	}
}

// keep strings import used in future tests
var _ = strings.HasPrefix
