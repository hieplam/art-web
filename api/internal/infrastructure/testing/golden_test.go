package testing_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	infratest "local/art-web/api/internal/infrastructure/testing"
)

// makeResp returns a fresh *http.Response so a single envelope can be both
// written (write mode) and compared (compare mode) in the same test.
func makeResp(status int, body string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Code = status
	rec.Header().Set("Content-Type", "application/json; charset=utf-8")
	rec.Body.WriteString(body)
	return rec.Result()
}

func TestAssertGolden_WriteThenCompare_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GOLDEN_DIR", dir)

	// 1) Write mode: AssertGolden writes the indented envelope to disk.
	t.Setenv("GOLDEN_UPDATE", "1")
	infratest.AssertGolden(t, "sample",
		makeResp(200, `{"id":"550e8400-e29b-41d4-a716-446655440000"}`))

	bs, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		t.Fatalf("expected golden file written: %v", err)
	}
	if !strings.Contains(string(bs), `<UUID>`) {
		t.Fatalf("golden file missing normalized UUID; got %s", bs)
	}
	if !strings.Contains(string(bs), `"body":`) ||
		!strings.Contains(string(bs), `"status":`) ||
		!strings.Contains(string(bs), `"headers":`) {
		t.Fatalf("envelope missing one of body/status/headers; got %s", bs)
	}

	// 2) Compare mode against the same input: must pass byte-strict.
	t.Setenv("GOLDEN_UPDATE", "")
	infratest.AssertGolden(t, "sample",
		makeResp(200, `{"id":"550e8400-e29b-41d4-a716-446655440000"}`))
}
