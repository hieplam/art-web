package dbtest_test

import (
	"encoding/base64"
	"net/http"
	"testing"

	"local/art-web/api/internal/dbtest"
)

func TestNormalize_ReplacesUUIDs(t *testing.T) {
	body := `{"id":"550e8400-e29b-41d4-a716-446655440000"}`
	got := dbtest.NormalizeBody([]byte(body))
	want := `{"id":"<UUID>"}`
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalize_ReplacesRFC3339Timestamps(t *testing.T) {
	body := `{"created_at":"2026-05-04T10:11:12Z"}`
	got := dbtest.NormalizeBody([]byte(body))
	want := `{"created_at":"<TIMESTAMP>"}`
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalize_ReplacesRFC3339NanoTimestamps(t *testing.T) {
	body := `{"created_at":"2026-05-04T10:11:12.123456789Z"}`
	got := dbtest.NormalizeBody([]byte(body))
	want := `{"created_at":"<TIMESTAMP>"}`
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalize_ReplacesHMACSignedURLSuffix(t *testing.T) {
	body := `{"url":"https://x/y.jpg?sig=deadbeefcafe1234&exp=1714823472"}`
	got := dbtest.NormalizeBody([]byte(body))
	want := `{"url":"https://x/y.jpg?sig=<SIG>&exp=<EXP>"}`
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalize_ReplacesBase64Cursor(t *testing.T) {
	// httpapi/artworks.go:54 builds cursors as base64.RawURLEncoding(timestamp|uuid).
	// Reproduce the actual encoding so the test matches what the real handler emits.
	rawCursor := base64.RawURLEncoding.EncodeToString(
		[]byte("2026-05-04T12:00:00.000000000Z|550e8400-e29b-41d4-a716-446655440000"))
	body := `{"next_cursor":"` + rawCursor + `","items":[]}`
	got := dbtest.NormalizeBody([]byte(body))
	want := `{"next_cursor":"<CURSOR>","items":[]}`
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalize_CursorIdempotent(t *testing.T) {
	body := `{"next_cursor":"<CURSOR>"}`
	got := dbtest.NormalizeBody([]byte(body))
	if string(got) != body {
		t.Fatalf("normalize must be idempotent on already-normalized cursor; got %q", got)
	}
}

func TestNormalize_NullCursor_Untouched(t *testing.T) {
	// When pagination is exhausted the handler emits next_cursor=null (not a
	// string). The cursor regex must not match null.
	body := `{"next_cursor":null,"items":[]}`
	got := dbtest.NormalizeBody([]byte(body))
	if string(got) != body {
		t.Fatalf("null cursor must be preserved; got %q", got)
	}
}

func TestNormalizeHeader_ReplacesAuthCookie(t *testing.T) {
	h := http.Header{}
	h.Add("Set-Cookie", "auth=abc.def.ghi; Path=/; HttpOnly; SameSite=Lax")
	dbtest.NormalizeHeaders(h)
	got := h.Get("Set-Cookie")
	want := "auth=<JWT>; Path=/; HttpOnly; SameSite=Lax"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestNormalizeHeader_EmptyAuthCookie_Preserved verifies the cookie-clearing
// path: ClearAuthCookie writes Set-Cookie: auth=; Max-Age=0; ... When the
// value is empty, [^;]+ does not match, so the regex leaves the cookie
// untouched. This is the correct contract behavior — an empty cleared value
// is part of the byte-strict snapshot, not noise to be normalized away.
func TestNormalizeHeader_EmptyAuthCookie_Preserved(t *testing.T) {
	h := http.Header{}
	h.Add("Set-Cookie", "auth=; Max-Age=0; Path=/; HttpOnly; SameSite=Lax")
	dbtest.NormalizeHeaders(h)
	got := h.Get("Set-Cookie")
	want := "auth=; Max-Age=0; Path=/; HttpOnly; SameSite=Lax"
	if got != want {
		t.Fatalf("empty-value cookie must be preserved verbatim; got %q want %q", got, want)
	}
}
