// api/internal/dbtest/golden.go
package dbtest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// goldenEnvelope is the on-disk shape of a snapshot. JSON keys are alphabetical
// so commit diffs stay reviewable.
type goldenEnvelope struct {
	Body    string              `json:"body"`
	Headers map[string][]string `json:"headers"`
	Status  int                 `json:"status"`
}

// AssertGolden compares resp (post-normalization) against a checked-in golden
// file. With GOLDEN_UPDATE=1 the file is (re)written instead of compared.
//
// Default golden directory: ./golden relative to the test file's package.
// Override via GOLDEN_DIR for unit tests of the helper itself.
//
// `name` must be a leaf filename (no path separators). The helper rejects names
// containing "/" so a typo can't silently write outside the golden directory.
func AssertGolden(t *testing.T, name string, resp *http.Response) {
	t.Helper()
	if filepath.Base(name) != name {
		t.Fatalf("golden name %q must not contain path separators", name)
	}

	defer resp.Body.Close()
	NormalizeHeaders(resp.Header)
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	bodyBytes = NormalizeBody(bodyBytes)

	envelope := goldenEnvelope{
		Status:  resp.StatusCode,
		Headers: pickHeaders(resp.Header),
		Body:    string(bodyBytes),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(envelope); err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	// json.Encoder.Encode appends a trailing newline; trim it so the file
	// content is identical to what json.MarshalIndent would produce.
	got := bytes.TrimRight(buf.Bytes(), "\n")

	dir := os.Getenv("GOLDEN_DIR")
	if dir == "" {
		dir = "golden"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name+".json")

	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (rerun with GOLDEN_UPDATE=1 to create)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden %s mismatch.\n--- got ---\n%s\n--- want ---\n%s\n", path, got, want)
	}
}

// pickHeaders selects only the headers the contract is anchored to: status-
// equivalent metadata (Content-Type, Cache-Control), Set-Cookie, and Location
// (for 302 redirect cells). Returning a sorted map keeps the on-disk envelope
// deterministic.
var snapshotHeaders = map[string]bool{
	"Content-Type":  true,
	"Cache-Control": true,
	"Set-Cookie":    true,
	"Location":      true,
}

func pickHeaders(h http.Header) map[string][]string {
	out := map[string][]string{}
	for k, vs := range h {
		if !snapshotHeaders[http.CanonicalHeaderKey(k)] {
			continue
		}
		out[k] = append([]string(nil), vs...)
	}
	// Sort header values within each key so multi-value headers don't drift.
	for _, vs := range out {
		sort.Strings(vs)
	}
	return out
}
