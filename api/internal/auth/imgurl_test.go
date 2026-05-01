// api/internal/auth/imgurl_test.go
package auth_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"local/art-web/api/internal/auth"
)

func TestPublicImageURL_NoSig(t *testing.T) {
	b := auth.NewURLBuilder("https://cdn.example.com", testKey, func() time.Time { return time.Unix(1700000000, 0) })
	got := b.Public("public/abc/0.jpg")
	if got != "https://cdn.example.com/img/public/abc/0.jpg" {
		t.Fatalf("got %s", got)
	}
}

func TestPrivateImageURL_HasSigAndExp(t *testing.T) {
	b := auth.NewURLBuilder("https://cdn.example.com", testKey, func() time.Time { return time.Unix(1700000000, 0) })
	got := b.Private("private/abc/0.jpg", 5*time.Minute)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.HasPrefix(u.Path, "/img/private/") {
		t.Fatalf("path %q missing /img/private prefix", u.Path)
	}
	if u.Query().Get("sig") == "" {
		t.Fatal("sig empty")
	}
	if u.Query().Get("exp") != "1700000300" {
		t.Fatalf("exp = %s want 1700000300", u.Query().Get("exp"))
	}
}

func TestPrivateImageURL_RejectsPublicKey(t *testing.T) {
	b := auth.NewURLBuilder("https://cdn.example.com", testKey, time.Now)
	if got := b.Private("public/abc/0.jpg", time.Minute); got != "" {
		t.Fatalf("expected empty for public key passed to Private(), got %s", got)
	}
}
