package postgres_test

import (
	"strings"
	"testing"

	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty input", "", "user"},
		{"whitespace only", "   ", "user"},
		{"basic ASCII", "Alice Smith", "alice-smith"},
		{"already lowercase", "bob", "bob"},
		{"trailing punctuation", "Alice!!!", "alice"},
		{"non-ASCII collapses", "Æl1ce 中", "l1ce"},
		{"length-cap exact 32", strings.Repeat("a", 32), strings.Repeat("a", 32)},
		{"length-cap truncates 33", strings.Repeat("a", 33), strings.Repeat("a", 32)},
		{"length-cap truncates 100", strings.Repeat("a", 100), strings.Repeat("a", 32)},
		{"only separators collapses", "---", "user"},
		{"unicode-only collapses", "中文", "user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := userpostgres.Slugify(tc.in)
			if got != tc.want {
				t.Fatalf("Slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
