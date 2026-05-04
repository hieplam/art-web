# Go backend refactor — Phase 0 implementation plan ("Test net")

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the safety net (≥ 80 % coverage on the four business slices + a byte-strict HTTP contract suite with locked golden snapshots) that lets the Phase 1 big-bang refactor land with zero observable side effects.

**Architecture:** 8 small, independently-mergeable PRs on `master`, in order. Each PR builds on the previous. PR 0.1 extends the testcontainers harness; 0.2–0.6 raise package coverage to ≥ 80 %; 0.7 introduces the HTTP contract matrix and the deterministic-ID injection points; 0.8 captures and locks the golden snapshots.

**Tech Stack:** Go, `testcontainers-go`, `golang-migrate`, `chi/v5`, `pgx/v5`, `httptest`. **No new runtime libraries land in Phase 0.** GORM, Wire, zerolog, validator are all Phase 1.

**Spec reference:** `docs/superpowers/specs/2026-05-04-go-backend-refactor-design.md`. When this plan and the spec disagree, the spec wins — file an issue and update the plan.

**Out of scope for this plan:** Phase 1 (the layout move + GORM/Wire/zerolog swap). Phase 1's exact validator-translator table and HTTP error mapping are *derived from* PR 0.8's captured snapshots, so Phase 1 gets its own plan written **after** PR 0.8 lands.

---

## File structure created or modified by this plan

The Phase 0 layout is the **current** flat layout — no slice-internal directories yet. Phase 0 changes only `api/internal/dbtest/` and `api/internal/httpapi/contract/` plus per-package `*_test.go` files.

| Path | Status | Responsibility |
|---|---|---|
| `api/internal/dbtest/postgres.go` | Modify | Existing testcontainers Postgres. Extend with `StartMinio(t)`. |
| `api/internal/dbtest/minio.go` | Create | New testcontainers MinIO harness shared with the contract suite. |
| `api/internal/dbtest/bootapp.go` | Create | `BootApp(t, opts) *http.Server` — boots the in-process API with deterministic clocks/RNG/IDProvider for the contract suite. |
| `api/internal/dbtest/normalize.go` | Create | Dynamic-byte normalizer (UUID, timestamp, JWT, HMAC, cursor regex replacers). |
| `api/internal/dbtest/golden.go` | Create | `assertGolden(t, name, resp)` helper with `GOLDEN_UPDATE=1` write-mode. |
| `api/internal/user/repo_test.go` | Modify | Add slugify, slug-collision, slug-exhaustion, ErrNotFound, lookupExistingOAuth tests. |
| `api/internal/user/slugify_test.go` | Create | Pure-function tests for `slugify` (currently unexported — see PR 0.2). |
| `api/internal/storage/localfs_test.go` | Modify | Add path-traversal-edge cases, missing-source for Move, **pin** `SignedURL` returns "unsupported" error. |
| `api/internal/storage/r2_test.go` | Modify | Add multipart, missing-bucket, content-type roundtrip. |
| `api/internal/image/handler_test.go` | Modify | Add `manifest_required`, `bad_multipart`, `file_count_mismatch`, content-type-roundtrip orphan tests. |
| `api/internal/image/service_test.go` | Modify (or create) | Add fingerprint-mismatch precedence, decode-format-vs-declared mismatch, max-bytes-exact-boundary tests. |
| `api/internal/httpapi/privacy_matrix_test.go` | Modify | Fill matrix gaps (anon GET on private 404, PATCH-of-other-user 404, DELETE-of-other-user 404). |
| `api/internal/httpapi/error_codes_test.go` | Create | One test per Shape-A and Shape-B error code listed in spec §3 — asserts status + body. |
| `api/internal/httpapi/devseed_test.go` | Modify | Cover `AppEnv != "test"` 404 path and the 200-success path body shape. |
| `api/internal/db/pool_test.go` | Modify | Add bad-DSN error path. |
| `api/internal/artwork/visibility_test.go` | Modify | Add storage-failure-during-flip, DB-tx-failure rollback, RollbackLog firing tests. |
| `api/internal/image/idprovider.go` | Create (PR 0.4) | `IDProvider` interface + `uuidIDProvider` default + `CounterIDProvider` for tests. |
| `api/internal/image/service.go` | Modify (PR 0.4) | Add `imageRepo` seam, `ids IDProvider` field, `NewServiceWithIDs`; replace `uuid.NewString()` at line 84 with `s.ids.NewID()`. |
| `api/internal/auth/handlers.go` | Modify (PR 0.7) | Replace `randState()` body with `RandReader.Read` injection (default `crypto/rand.Reader`). |
| `api/internal/auth/randreader.go` | Create (PR 0.7) | `RandReader` interface + default + deterministic test impl. |
| `api/internal/httpapi/devseed.go` | Modify (PR 0.7) | Accept optional `?suffix=` query param so contract suite gets deterministic slug suffixes. |
| `api/internal/httpapi/contract/doc.go` | Create | Empty package created in PR 0.1 so `make test-contract` compiles before PR 0.7. |
| `api/internal/httpapi/contract/placeholder_test.go` | Create | Trivial test in PR 0.1; replaced by matrix_test.go in PR 0.7. |
| `api/internal/httpapi/contract/matrix_test.go` | Create | Cartesian-product HTTP contract suite (PR 0.7 — replaces placeholder). |
| `api/internal/httpapi/contract/forbidden_status_test.go` | Create | Meta-test: scans `golden/*.json` for forbidden statuses (403, translated 409). |
| `api/internal/httpapi/contract/fake_provider.go` | Create | Deterministic `auth.Provider` for the contract suite (PR 0.7). |
| `api/internal/httpapi/contract/golden/` | Create | Per-test JSON snapshot files. Empty until PR 0.8. |
| `api/Makefile` | Modify | Add `test-contract`, `test-cover`, `test-cover-slices` targets. |
| `.github/workflows/contract.yml` (or equivalent) | Create | CI job `contract_suite`. Skipped initially (PR 0.1), required after PR 0.8. |
| `CODEOWNERS` (root) | Modify | Protect `api/internal/httpapi/contract/golden/`. |

---

## Conventions for every PR in this plan

- **Branch naming:** `phase0/0.N-<short-name>` (e.g. `phase0/0.1-test-harness`).
- **Commit message format (per `~/.claude/rules/git-conventions.md`):** every commit subject starts with `[<branch-name-without-prefix>]`. Examples:
  - `[0.1-test-harness] feat: add testcontainers MinIO module`
  - `[0.2-user] test: cover slug-exhaustion error path`
- **Frequent commits:** commit after every passing test or every couple of related tests, not just at end of PR.
- **No co-author / Claude attribution footer** (per the project's git rules).
- **Bug-fix policy (spec §5.6):** if a Phase 0 test surfaces a real bug, **stop**, ship the fix as a separate `phase-0-behavior-change` PR with reviewer signoff, then resume.
- **Coverage check command per slice (Phase 0 layout):**
  ```bash
  go test -cover ./internal/<slice>/...
  ```

---

## PR 0.1 — Test-harness scaffolding

**Branch:** `phase0/0.1-test-harness`

**Goal:** Extend `internal/dbtest/` so PR 0.7 can boot the full API in-process with deterministic sources and capture byte-strict goldens. Wire a `contract_suite` CI job that is skipped today and gets activated in PR 0.8.

**Acceptance criteria:**
- All current tests still pass (`make test` green).
- New `dbtest.StartMinio(t)` boots a MinIO container and returns endpoint/access keys.
- New `dbtest.BootApp(t, opts)` returns an `*http.Server` that the contract suite can call.
- New `dbtest.AssertGolden(t, name, resp)` exists, with `GOLDEN_UPDATE=1` write mode.
- New `dbtest.Normalize(body, headers)` rewrites dynamic bytes per spec §5.3.2.
- A self-test in `dbtest/bootapp_test.go` boots the API and asserts `GET /healthz` returns 200.
- CI workflow `contract.yml` exists with the `contract_suite` job; it currently does nothing more than `echo "skipped until PR 0.8"`.

### Task 1.1 — Add MinIO testcontainers harness

**Files:**
- Create: `api/internal/dbtest/minio.go`

- [ ] **Step 1: Add the testcontainers MinIO module dependency.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go get github.com/testcontainers/testcontainers-go/modules/minio@latest
go mod tidy
```

- [ ] **Step 2: Create the harness file.**

Write `api/internal/dbtest/minio.go`:

```go
// api/internal/dbtest/minio.go
package dbtest

import (
	"context"
	"sync"
	"testing"
	"time"

	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

type MinioInfo struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	Bucket    string
}

var (
	minioOnce sync.Once
	minioInfo MinioInfo
	minioErr  error
)

// StartMinio boots a single MinIO container shared across all tests in the run
// (sync.Once mirrors StartPostgres). Tests must not run in parallel with other
// tests that mutate buckets.
func StartMinio(t testing.TB) MinioInfo {
	t.Helper()
	minioOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		container, err := tcminio.Run(ctx, "minio/minio:latest",
			tcminio.WithUsername("minioadmin"),
			tcminio.WithPassword("minioadmin"),
		)
		if err != nil {
			minioErr = err
			return
		}
		endpoint, err := container.ConnectionString(ctx)
		if err != nil {
			minioErr = err
			return
		}
		minioInfo = MinioInfo{
			Endpoint:  endpoint,
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Bucket:    "artweb-test",
		}
	})
	if minioErr != nil {
		t.Fatalf("minio harness: %v", minioErr)
	}
	return minioInfo
}
```

- [ ] **Step 3: Add a smoke test for the harness.**

Write `api/internal/dbtest/minio_test.go`:

```go
package dbtest_test

import (
	"testing"

	"local/art-web/api/internal/dbtest"
)

func TestStartMinio_ReturnsEndpoint(t *testing.T) {
	info := dbtest.StartMinio(t)
	if info.Endpoint == "" {
		t.Fatal("StartMinio returned empty endpoint")
	}
	if info.Bucket == "" {
		t.Fatal("StartMinio returned empty bucket")
	}
}
```

- [ ] **Step 4: Run the smoke test.**

Run: `go test ./internal/dbtest/... -run TestStartMinio_ReturnsEndpoint -v`
Expected: PASS (container boots in ~10–30 s on cold cache).

- [ ] **Step 5: Commit.**

```bash
git add api/internal/dbtest/minio.go api/internal/dbtest/minio_test.go api/go.mod api/go.sum
git commit -m "[0.1-test-harness] feat: add testcontainers MinIO harness"
```

### Task 1.2 — Add the dynamic-byte normalizer

**Files:**
- Create: `api/internal/dbtest/normalize.go`
- Create: `api/internal/dbtest/normalize_test.go`

- [ ] **Step 1: Write the failing test.**

Write `api/internal/dbtest/normalize_test.go`:

```go
package dbtest_test

import (
	"encoding/base64"
	"net/http"
	"strings"
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
	if !strings.Contains(got, "auth=<JWT>") {
		t.Fatalf("expected auth=<JWT>, got %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail with "package not found".**

Run: `go test ./internal/dbtest/... -run TestNormalize -v`
Expected: FAIL — `dbtest.NormalizeBody` and `dbtest.NormalizeHeaders` are undefined.

- [ ] **Step 3: Implement the normalizer.**

Write `api/internal/dbtest/normalize.go`:

```go
// api/internal/dbtest/normalize.go
package dbtest

import (
	"net/http"
	"regexp"
)

// Patterns are documented in spec §5.3.2. Order matters; see NormalizeBody.
var (
	// nextCursorRe matches the JSON field `"next_cursor":"<base64-RawURL>"`
	// and rewrites the value to <CURSOR>. The base64.RawURLEncoding alphabet
	// is [A-Za-z0-9_-] (no padding). The regex is anchored to the field name so
	// arbitrary base64-looking strings elsewhere in the body are NOT replaced.
	// `null` cursors are not matched (the regex requires a quoted string).
	nextCursorRe = regexp.MustCompile(`"next_cursor":"[A-Za-z0-9_\-]+"`)
	uuidRe       = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	rfc3339Re    = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)
	signedURLRe  = regexp.MustCompile(`\?sig=[0-9a-f]+&exp=\d+`)
	cookieAuthRe = regexp.MustCompile(`auth=[^;]+`)
	jwtBearerRe  = regexp.MustCompile(`Bearer [A-Za-z0-9_\-\.]+`)
)

// NormalizeBody rewrites every non-deterministic byte sequence in body to a
// stable placeholder so post-normalization snapshots compare equal.
//
// Order matters:
//   1. nextCursorRe runs first so the opaque base64 cursor is collapsed before
//      any later rule could match digits or hyphens inside it.
//   2. signedURLRe runs before rfc3339Re because the signed-URL `exp=` integer
//      would otherwise be left as a bare digit run.
//   3. rfc3339Re runs before uuidRe because timestamps' digit groups don't
//      overlap with the UUID pattern, but explicit ordering documents intent.
func NormalizeBody(body []byte) []byte {
	body = nextCursorRe.ReplaceAll(body, []byte(`"next_cursor":"<CURSOR>"`))
	body = signedURLRe.ReplaceAll(body, []byte(`?sig=<SIG>&exp=<EXP>`))
	body = rfc3339Re.ReplaceAll(body, []byte(`<TIMESTAMP>`))
	body = uuidRe.ReplaceAll(body, []byte(`<UUID>`))
	return body
}

// NormalizeHeaders rewrites Set-Cookie and Authorization headers in place.
func NormalizeHeaders(h http.Header) {
	if vals, ok := h["Set-Cookie"]; ok {
		for i, v := range vals {
			vals[i] = cookieAuthRe.ReplaceAllString(v, "auth=<JWT>")
		}
	}
	if vals, ok := h["Authorization"]; ok {
		for i, v := range vals {
			vals[i] = jwtBearerRe.ReplaceAllString(v, "Bearer <JWT>")
		}
	}
}
```

- [ ] **Step 4: Run the tests to verify pass.**

Run: `go test ./internal/dbtest/... -run TestNormalize -v`
Expected: PASS for all six subtests.

- [ ] **Step 5: Commit.**

```bash
git add api/internal/dbtest/normalize.go api/internal/dbtest/normalize_test.go
git commit -m "[0.1-test-harness] feat: add dynamic-byte normalizer for snapshots"
```

### Task 1.3 — Add the golden-file helper

**Files:**
- Create: `api/internal/dbtest/golden.go`
- Create: `api/internal/dbtest/golden_test.go`

- [ ] **Step 1: Write the failing test.**

Write `api/internal/dbtest/golden_test.go`:

```go
package dbtest_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local/art-web/api/internal/dbtest"
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
	dbtest.AssertGolden(t, "sample",
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
	dbtest.AssertGolden(t, "sample",
		makeResp(200, `{"id":"550e8400-e29b-41d4-a716-446655440000"}`))
}

```

> The failure-path branch (compare-mode mismatch → t.Fatalf) is not unit-tested. Catching `t.Fatalf` requires a custom test interface that adds noise to the helper signature; the failure path is exercised organically in PR 0.8 (intentional snapshot mismatches surface immediately) and afterwards by Phase 1 review. If a regression in the failure path is a concern, add a separate `*_failure_test.go` later that uses `runtime.Goexit` capture, but defer for now.

- [ ] **Step 2: Run to confirm failure.**

Run: `go test ./internal/dbtest/... -run TestAssertGolden -v`
Expected: FAIL — `dbtest.AssertGolden` undefined.

- [ ] **Step 3: Implement the helper.**

Write `api/internal/dbtest/golden.go`:

```go
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
func AssertGolden(t *testing.T, name string, resp *http.Response) {
	t.Helper()

	NormalizeHeaders(resp.Header)
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyBytes = NormalizeBody(bodyBytes)

	envelope := goldenEnvelope{
		Status:  resp.StatusCode,
		Headers: pickHeaders(resp.Header),
		Body:    string(bodyBytes),
	}
	got, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

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
// equivalent metadata (Content-Type, Cache-Control) and Set-Cookie. Returning
// a sorted map keeps the on-disk envelope deterministic.
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
```

- [ ] **Step 4: Run tests to verify pass.**

Run: `go test ./internal/dbtest/... -run TestAssertGolden -v`
Expected: PASS for both subtests.

- [ ] **Step 5: Commit.**

```bash
git add api/internal/dbtest/golden.go api/internal/dbtest/golden_test.go
git commit -m "[0.1-test-harness] feat: add golden-file assert helper"
```

### Task 1.4 — Add the in-process API booter

**Files:**
- Create: `api/internal/dbtest/bootapp.go`
- Create: `api/internal/dbtest/bootapp_test.go`

- [ ] **Step 1: Write the booter.**

The booter accepts injectable test fakes (clock, RNG, IDs). PR 0.7 adds the actual `IDProvider`/`RandReader` types — for now, BootApp accepts `time.Time` clocks only and falls back to current behavior for IDs. This task lays the structure so PR 0.7 adds two parameters cleanly.

Write `api/internal/dbtest/bootapp.go`:

```go
// api/internal/dbtest/bootapp.go
package dbtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/db"
	"local/art-web/api/internal/httpapi"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
)

// BootOpts injects deterministic sources for the contract suite. Any zero
// field falls back to the current production default.
type BootOpts struct {
	// FixedNow, when non-zero, replaces every JWT/URL clock with a constant.
	FixedNow time.Time

	// AppEnv defaults to "test" so the dev seed route is registered.
	AppEnv string

	// Frontend defaults to "http://localhost:3000/".
	Frontend string

	// AllowedOrigin defaults to "http://localhost:3000".
	AllowedOrigin string

	// JWTKey / SignKey default to fixed 32-byte test keys.
	JWTKey  []byte
	SignKey []byte

	// Providers, when non-nil, is the auth.Provider map handed to httpapi.New.
	// Tests inject a deterministic fake here so /auth/{provider}/start emits a
	// real 302 redirect (default empty map → unknown_provider 404, which would
	// lock the wrong contract bytes — see Finding 4 of the round-1 review).
	Providers map[string]auth.Provider
}

// BootApp returns an *httptest.Server backed by the real httpapi.Deps stack
// against a fresh testcontainers Postgres + a local-filesystem store under
// t.TempDir(). It mirrors httpapi/testutil_test.go's setup but is exported so
// the contract suite can use it.
func BootApp(t testing.TB, opts BootOpts) *httptest.Server {
	t.Helper()

	if opts.AppEnv == "" {
		opts.AppEnv = "test"
	}
	if opts.Frontend == "" {
		opts.Frontend = "http://localhost:3000/"
	}
	if opts.AllowedOrigin == "" {
		opts.AllowedOrigin = "http://localhost:3000"
	}
	if len(opts.JWTKey) == 0 {
		opts.JWTKey = []byte("test-jwt-key-pad-to-32-bytes!!!!")
	}
	if len(opts.SignKey) == 0 {
		opts.SignKey = []byte("test-sign-key-pad-to-32-bytes!!!")
	}

	dsn := StartPostgres(t)
	pool, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})

	store := storage.NewLocalFS(t.TempDir())
	clock := time.Now
	if !opts.FixedNow.IsZero() {
		clock = func() time.Time { return opts.FixedNow }
	}
	jwts := auth.NewJWT(opts.JWTKey, clock)
	urls := auth.NewURLBuilder("http://localhost:8787", opts.SignKey, clock)
	arts := artwork.NewRepo(pool)
	images := image.NewRepo(pool)
	imgSvc := image.NewService(store, images, arts)
	upload := image.NewHandler(imgSvc, arts, urls)
	vis := artwork.NewVisibilityService(arts, store)

	providers := opts.Providers
	if providers == nil {
		providers = map[string]auth.Provider{}
	}
	router := httpapi.New(&httpapi.Deps{
		AppEnv:        opts.AppEnv,
		JWT:           jwts,
		URL:           urls,
		Providers:     providers,
		Users:         user.NewRepo(pool),
		Artworks:      arts,
		Tags:          artwork.NewTagsRepo(pool),
		Images:        images,
		Store:         store,
		Upload:        upload,
		Vis:           vis,
		Frontend:      opts.Frontend,
		AllowedOrigin: opts.AllowedOrigin,
		CookieOpts:    auth.CookieOpts{Secure: false},
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

// StatusOf is a convenience for callers that only need the status code.
func StatusOf(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}
```

- [ ] **Step 2: Write the smoke test.**

Write `api/internal/dbtest/bootapp_test.go`:

```go
package dbtest_test

import (
	"net/http"
	"testing"

	"local/art-web/api/internal/dbtest"
)

func TestBootApp_HealthzReturns200(t *testing.T) {
	srv := dbtest.BootApp(t, dbtest.BootOpts{})

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
}
```

- [ ] **Step 3: Run.**

Run: `go test ./internal/dbtest/... -run TestBootApp_HealthzReturns200 -v`
Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/dbtest/bootapp.go api/internal/dbtest/bootapp_test.go
git commit -m "[0.1-test-harness] feat: add BootApp helper for contract suite"
```

### Task 1.4b — Create placeholder contract package

The contract package itself is filled in by PR 0.7 / 0.8. PR 0.1 only creates an empty package + one trivial test so `make test-contract` (added in Task 1.5) compiles immediately rather than erroring on a missing path.

**Files:**
- Create: `api/internal/httpapi/contract/doc.go`
- Create: `api/internal/httpapi/contract/placeholder_test.go`

- [ ] **Step 1: Create the package marker.**

Write `api/internal/httpapi/contract/doc.go`:

```go
// Package contract holds the byte-strict HTTP contract suite. PR 0.7 fills in
// the cartesian matrix, PR 0.8 captures golden snapshots, and a forbidden-
// status meta-test guards against contract drift in Phase 1.
package contract
```

- [ ] **Step 2: Create the placeholder test.**

Write `api/internal/httpapi/contract/placeholder_test.go`:

```go
package contract_test

import "testing"

// TestPlaceholder keeps `make test-contract` green until PR 0.7 replaces this
// file with the real matrix. Delete this file as the first step of PR 0.7.
func TestPlaceholder(t *testing.T) {}
```

- [ ] **Step 3: Verify the target works.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go test ./internal/httpapi/contract/... -v
```

Expected: `--- PASS: TestPlaceholder` and `ok local/art-web/api/internal/httpapi/contract`.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/httpapi/contract/doc.go api/internal/httpapi/contract/placeholder_test.go
git commit -m "[0.1-test-harness] feat: scaffold contract package (placeholder until PR 0.7)"
```

### Task 1.5 — Add Makefile targets and CI stub

**Files:**
- Modify: `api/Makefile`
- Create: `.github/workflows/contract.yml` (or whatever workflow tool the repo uses — verify with `ls .github/workflows/`)

- [ ] **Step 1: Confirm CI tooling.**

```bash
ls /Users/todd.lam/WORK/_TestScripts/art-web/.github/workflows/ 2>/dev/null || echo "no workflows yet"
```

If no workflows exist, defer the CI step to the team and note the manual fallback in the PR description: "Run `make test-contract` locally before merging until CI is added."

- [ ] **Step 2: Extend the Makefile.**

Replace the contents of `api/Makefile` with:

```make
.PHONY: tidy build test test-race lint test-contract test-cover test-cover-slices

tidy:               ; go mod tidy
build:              ; go build -o bin/api ./cmd/api
test:               ; go test -coverprofile=/tmp/coverage.out ./...  && go tool cover --func=/tmp/coverage.out
test-race:          ; go test -race ./...
lint:               ; go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...

# Phase 0 contract suite. Runs only the contract package; no goldens until PR 0.8.
test-contract:      ; go test ./internal/httpapi/contract/... -v

# Phase 0 coverage on the four business slices (gate: ≥ 80% statement coverage each).
test-cover-slices:
	@for pkg in auth user artwork image; do \
	  echo "=== $$pkg ===" ; \
	  go test -coverprofile=/tmp/cov-$$pkg.out ./internal/$$pkg/... ; \
	  go tool cover -func=/tmp/cov-$$pkg.out | tail -1 ; \
	done

test-cover:
	go test -coverprofile=/tmp/coverage.out ./...
	go tool cover -func=/tmp/coverage.out | tail -20
```

- [ ] **Step 3: Run the new targets to confirm they work.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
make test-cover-slices 2>&1 | tail -10
```

Expected: four `total: (statements) NN.N%` lines printed (matching spec §3 baseline numbers ±1 %).

- [ ] **Step 4: (If CI exists) add the contract-suite job stub.**

If `.github/workflows/` exists, add a new file `.github/workflows/contract.yml`:

```yaml
name: contract

on:
  pull_request:
    paths:
      - 'api/**'
  push:
    branches: [master]

jobs:
  contract_suite:
    runs-on: ubuntu-latest
    if: false   # disabled until PR 0.8 captures goldens
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          # Pin to whatever api/go.mod declares (currently go 1.25.0). Don't
          # hardcode a version — go.mod drift would silently break CI.
          go-version-file: api/go.mod
      - name: contract
        run: cd api && make test-contract
```

- [ ] **Step 5: Commit.**

```bash
git add api/Makefile .github/workflows/contract.yml
git commit -m "[0.1-test-harness] chore: add Makefile targets and disabled contract CI job"
```

### Task 1.6 — Open the PR

- [ ] **Step 1: Push and open the PR.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web
git push -u origin phase0/0.1-test-harness
gh pr create --base master --title "[0.1-test-harness] Test harness scaffolding" --body "$(cat <<'EOF'
## Summary
- Add testcontainers MinIO module (`dbtest.StartMinio`).
- Add dynamic-byte normalizer (UUID, RFC3339, signed-URL HMAC, JWT cookie).
- Add `dbtest.AssertGolden` with `GOLDEN_UPDATE=1` write mode.
- Add `dbtest.BootApp` so PR 0.7 can boot the API in-process.
- Add Makefile targets: `test-contract`, `test-cover-slices`, `test-cover`.
- Add disabled `contract_suite` CI job; activated in PR 0.8.

## Test plan
- [ ] `make test` green
- [ ] `make test-cover-slices` prints baseline numbers
- [ ] `make test-contract` no-op-passes (no goldens yet)

Refs spec: docs/superpowers/specs/2026-05-04-go-backend-refactor-design.md §5.1
EOF
)"
```

---

## PR 0.2 — `user` 63.8 → 80 %

**Branch:** `phase0/0.2-user`

**Goal:** Cover the actual surface area of `internal/user/repo.go`. Per spec §8.1, this means slugify edges, slug-collision retry path, slug-exhaustion error, OAuth-uniqueness re-read path, ErrNotFound mapping, GetBySlug pass-through, non-23505 Postgres pass-through. **Excludes:** profile-update tests (no PATCH route exists), soft-delete tests (no `deleted_at` column).

**Acceptance criteria:**
- `go test -cover ./internal/user/...` reports ≥ 80 % statement coverage.
- All new tests use `dbtest.StartPostgres` (not their own container).

### Task 2.1 — Cover slugify edge cases

**Files:**
- Modify: `api/internal/user/repo.go` (export `Slugify` for tests)
- Create: `api/internal/user/slugify_test.go`

- [ ] **Step 1: Export `slugify` to `Slugify`.**

`slugify` is currently unexported (`api/internal/user/repo.go:31`). Renaming it to `Slugify` is a no-op refactor — there is no other call site (verified by `grep -r "user.slugify\|user\\.Slugify" api/`).

Open `api/internal/user/repo.go` and:

```
- func slugify(s string) string {
+ func Slugify(s string) string {
```

Then update line 57 in the same file:

```
- base := slugify(displayName)
+ base := Slugify(displayName)
```

- [ ] **Step 2: Write the slugify table test.**

Write `api/internal/user/slugify_test.go`:

```go
package user_test

import (
	"strings"
	"testing"

	"local/art-web/api/internal/user"
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
			got := user.Slugify(tc.in)
			if got != tc.want {
				t.Fatalf("Slugify(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run.**

Run: `go test ./internal/user/... -run TestSlugify -v`
Expected: PASS for all subtests.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/user/repo.go api/internal/user/slugify_test.go
git commit -m "[0.2-user] test: export Slugify and cover edge cases"
```

### Task 2.2 — Cover slug-exhaustion error

**Files:**
- Modify: `api/internal/user/repo_test.go`

- [ ] **Step 1: Read the existing helper to see the available shape.**

Already read in this session: `newRepo(t)` returns `*user.Repo`, plus three working tests use `r.UpsertOAuth`. Slug exhaustion is the loop at `repo.go:59-83` — after 50 conflicting slugs it returns `errors.New("slug exhausted")`.

To trigger it without 50 hand-rolled rows, seed 50 users whose slugs collide with `Slugify("alice")` = `"alice"`, `"alice-2"`, ..., `"alice-50"`. The 51st upsert with the same display name must error with "slug exhausted".

- [ ] **Step 2: Append the test to `api/internal/user/repo_test.go`:**

```go
func TestUpsertOAuth_SlugExhaustionAfter50Collisions(t *testing.T) {
	r := newRepo(t)
	// Seed 50 users that occupy the slug space "alice", "alice-2", ..., "alice-50".
	for i := 0; i < 50; i++ {
		subject := "S" + strings.Repeat("x", i+1) // unique oauth_subject per insertion
		if _, err := r.UpsertOAuth(t.Context(), "google", subject, "x@x", "alice", ""); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// 51st upsert must run out of slug space and return the sentinel error.
	_, err := r.UpsertOAuth(t.Context(), "google", "exhausted-subject", "x@x", "alice", "")
	if err == nil {
		t.Fatal("expected slug-exhaustion error")
	}
	if !strings.Contains(err.Error(), "slug exhausted") {
		t.Fatalf("expected 'slug exhausted', got %v", err)
	}
}
```

- [ ] **Step 3: Run.**

Run: `go test ./internal/user/... -run TestUpsertOAuth_SlugExhaustionAfter50Collisions -v`
Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/user/repo_test.go
git commit -m "[0.2-user] test: cover slug-exhaustion error after 50 collisions"
```

### Task 2.3 — Cover ErrNotFound, GetBySlug pass-through, OAuth uniqueness re-read

**Files:**
- Modify: `api/internal/user/repo_test.go`

- [ ] **Step 1: Append three more tests to `api/internal/user/repo_test.go`:**

```go
func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	r := newRepo(t)
	_, err := r.Get(t.Context(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetBySlug_NotFound_PassesThroughError(t *testing.T) {
	r := newRepo(t)
	_, err := r.GetBySlug(t.Context(), "no-such-slug")
	if err == nil {
		t.Fatal("expected error for missing slug")
	}
	// Current behavior: GetBySlug does NOT translate to ErrNotFound — it returns
	// the raw pgx.ErrNoRows. This test pins that behavior so it does not change
	// silently.
	if errors.Is(err, user.ErrNotFound) {
		t.Fatalf("GetBySlug should not return ErrNotFound; got %v", err)
	}
}

func TestUpsertOAuth_DuplicateOAuthKey_ReturnsExistingID(t *testing.T) {
	r := newRepo(t)

	first, err := r.UpsertOAuth(t.Context(), "google", "S-DUP", "a@b", "alice-dup", "")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Re-upsert with the SAME provider+subject but different display name → must
	// hit the lookupExistingOAuth fast path at repo.go:45-55 (UPDATE, no INSERT).
	second, err := r.UpsertOAuth(t.Context(), "google", "S-DUP", "a@b", "different-name", "")
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if first != second {
		t.Fatalf("expected same id; got %s vs %s", first, second)
	}
}
```

Add `"errors"` to the imports if not already present.

- [ ] **Step 2: Run.**

Run: `go test ./internal/user/... -run 'TestGet_NotFound|TestGetBySlug_NotFound|TestUpsertOAuth_DuplicateOAuthKey' -v`
Expected: PASS for all three.

- [ ] **Step 3: Verify coverage threshold.**

```bash
go test -cover ./internal/user/...
```

Expected: a single line ending `coverage: NN.N% of statements` where `NN.N >= 80.0`. If less, identify the uncovered branch with `go test -coverprofile=/tmp/u.out ./internal/user/... && go tool cover -html=/tmp/u.out` and add a focused test before continuing.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/user/repo_test.go
git commit -m "[0.2-user] test: cover ErrNotFound, GetBySlug, OAuth re-read paths"
```

### Task 2.4 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.2-user
gh pr create --base master --title "[0.2-user] Lift user package coverage to ≥ 80%" --body "$(cat <<'EOF'
## Summary
- Export `slugify` → `Slugify`; cover edge cases (empty, length cap, non-ASCII).
- Cover slug-exhaustion error after 50 collisions.
- Cover `Get(404)→ErrNotFound`, `GetBySlug(404)` pass-through, OAuth-uniqueness re-read.

## Test plan
- [ ] `go test -cover ./internal/user/...` reports ≥ 80 %
- [ ] All slugify subtests pass
- [ ] No new dependencies

Refs spec: §8.1 PR 0.2.
EOF
)"
```

---

## PR 0.3 — `storage` 69.1 → 80 %

**Branch:** `phase0/0.3-storage`

**Goal:** Per spec §8.1: r2 multipart edge cases, localfs path-traversal guard, missing-bucket error, content-type round-trip.

**Acceptance criteria:** `go test -cover ./internal/storage/...` ≥ 80 %.

### Task 3.1 — Identify uncovered lines

- [ ] **Step 1: Generate a coverage report.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go test -coverprofile=/tmp/storage.out ./internal/storage/...
go tool cover -func=/tmp/storage.out | grep -v "100.0%"
```

The non-100 % lines tell you exactly which branches need new tests. Confirm coverage of: `Move(missing-source)`, `SignedURL` round-trip, `r2.Put` for files larger than the multipart threshold, `Delete(missing-key)`.

### Task 3.2 — Cover localfs edge cases

**Files:**
- Modify: `api/internal/storage/localfs_test.go`

- [ ] **Step 1: Append the new tests:**

```go
func TestLocalFS_Move_MissingSource_ReturnsError(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	if err := s.Move(context.Background(), "no/such/key.jpg", "dst/key.jpg"); err == nil {
		t.Fatal("expected error when moving a missing source")
	}
}

func TestLocalFS_Delete_MissingKey_NoError(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	// localfs treats Delete-of-missing as a no-op so callers can call it
	// idempotently as part of orphan cleanup. Pin that contract.
	if err := s.Delete(context.Background(), "no/such/key.jpg"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestLocalFS_Exists_FalseForMissing(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ok, err := s.Exists(context.Background(), "no/such/key")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestLocalFS_SignedURL_ReturnsUnsupported(t *testing.T) {
	// Pin the current contract: localfs does NOT implement SignedURL — it
	// returns ("", error). Verified against api/internal/storage/localfs.go:
	//   return "", errors.New("localfs does not support signed URLs")
	// HTTP-level signed URLs come from auth.URLBuilder, not the storage adapter.
	s := storage.NewLocalFS(t.TempDir())
	url, err := s.SignedURL(context.Background(), "abc/0.jpg", 5*time.Minute)
	if err == nil {
		t.Fatal("expected localfs.SignedURL to return an error")
	}
	if url != "" {
		t.Fatalf("expected empty URL on error, got %q", url)
	}
	if !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("expected unsupported message, got %v", err)
	}
}

func TestLocalFS_RejectsTraversal_Variants(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	bad := []string{"../etc/passwd", "abc/../../etc/passwd", "abc/./../../etc/passwd"}
	for _, k := range bad {
		if err := s.Put(context.Background(), k, strings.NewReader("x"), "text/plain"); err == nil {
			t.Fatalf("expected traversal rejection for %q", k)
		}
	}
}
```

Add `"time"` to imports if not present.

- [ ] **Step 2: Run.**

Run: `go test ./internal/storage/... -run 'TestLocalFS_Move_MissingSource|TestLocalFS_Delete_MissingKey|TestLocalFS_Exists_False|TestLocalFS_SignedURL|TestLocalFS_RejectsTraversal_Variants' -v`
Expected: PASS for all.

- [ ] **Step 3: Commit.**

```bash
git add api/internal/storage/localfs_test.go
git commit -m "[0.3-storage] test: cover localfs missing-source, signed-URL, traversal variants"
```

### Task 3.3 — Cover R2 paths

**Files:**
- Modify: `api/internal/storage/r2_test.go`

- [ ] **Step 1: Read the existing R2 test to understand the pattern.**

```bash
sed -n '1,40p' /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/storage/r2_test.go
```

Note the existing minio/testcontainers wiring. Your new tests reuse that helper.

- [ ] **Step 2: Append tests for content-type round-trip and missing-bucket error.**

The exact code depends on the existing helper signature; below is the template — adapt the helper-call line to match what's in the file.

```go
func TestR2_PutGet_ContentTypeRoundtrip(t *testing.T) {
	s := newR2(t)            // existing helper — adapt name if different
	ctx := context.Background()
	body := bytes.NewReader([]byte("payload"))
	if err := s.Put(ctx, "abc/0.jpg", body, "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	r, err := s.Get(ctx, "abc/0.jpg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != "payload" {
		t.Fatalf("got %q want payload", got)
	}
}

func TestR2_Delete_MissingKey_NoError(t *testing.T) {
	s := newR2(t)
	if err := s.Delete(context.Background(), "no/such/key"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
```

If a "missing bucket" path exists in `r2.go` (read it: `cat api/internal/storage/r2.go`), add a test that constructs the adapter with a non-existent bucket name and asserts `Put` returns a typed error. If the adapter creates the bucket lazily, skip this test and note it in the PR description.

- [ ] **Step 3: Run, then check the slice coverage.**

```bash
go test ./internal/storage/... -v
go test -cover ./internal/storage/...
```

Expected: PASS, `coverage: NN.N% of statements` ≥ 80.0.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/storage/r2_test.go
git commit -m "[0.3-storage] test: cover R2 content-type roundtrip + missing-key Delete"
```

### Task 3.4 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.3-storage
gh pr create --base master --title "[0.3-storage] Lift storage package coverage to ≥ 80%" --body "$(cat <<'EOF'
## Summary
- Cover localfs missing-source Move, missing-key Delete idempotency, signed-URL roundtrip, traversal variants.
- Cover R2 content-type roundtrip and missing-key Delete.

## Test plan
- [ ] `go test -cover ./internal/storage/...` reports ≥ 80 %

Refs spec §8.1 PR 0.3.
EOF
)"
```

---

## PR 0.4 — `image` 69.7 → 80 %

**Branch:** `phase0/0.4-image`

**Goal:** Per spec §8.1: orphan cleanup race, content-type mismatch, signed-URL expiry boundary, blurhash error path.

**Acceptance criteria:** `go test -cover ./internal/image/...` ≥ 80 %.

### Task 4.1 — Identify uncovered lines

- [ ] **Step 1: Generate a coverage report.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go test -coverprofile=/tmp/image.out ./internal/image/...
go tool cover -func=/tmp/image.out | grep -v "100.0%"
```

The handler tests already cover most of the handler surface (per the existing `handler_test.go`). The gap is most likely in `service.go` (the fingerprint-mismatch order, the orphan-cleanup-on-Insert-failure branch at line 100, the idempotent-insert-but-row-not-found error at line 109–111, and the storage-Put-failure branch).

### Task 4.2 — Cover the unhandled-error paths in `image/service.go`

**Files:**
- Create: `api/internal/image/service_test.go` (if it does not exist — check first with `ls /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/image/`)
- Modify: `api/internal/image/handler_test.go` (if `service_test.go` would duplicate scaffolding; preferred form is its own file)

The current `handler_test.go` already covers fingerprint-mismatch, idempotent retry, content-type mismatch (422), position conflict + orphan cleanup, and partial-failure resume. The remaining branches in `service.go` not covered today:

1. **Storage `Put` returns an error** (line 88) — needs `failingStore` returning err on Put #1.
2. **`images.Insert` returns an error and orphan cleanup fires** (line 100) — needs an Insert that fails after a successful Put.
3. **Idempotent insert says "Existed=true" but `FindByClientImageID` returns nil** (line 109–111) — race-window error path; mock the Repo.

These need `Service`-level tests with a stubbed `*Repo` interface, but the current code uses concrete `*image.Repo`. Two options:

- **Option A (preferred for Phase 0):** add a small interface in `service.go` (no behavior change) and inject the existing repo. **Counts as a `phase-0-behavior-change` PR** if it touches non-test code beyond test seams.
- **Option B:** use a real Postgres but provoke the failure modes via SQL state corruption. Brittle. Avoid.

Choose Option A. The interface is internal and only `*Repo` satisfies it today; production wiring is unchanged.

> **Sequencing note (round-2 Finding 3):** PR 0.7 will swap `uuid.NewString()` at `service.go:84` for an injected `IDProvider`. If `IDProvider` is introduced only then, the service test literals added below in PR 0.4 would set `ids = nil` and panic when PR 0.7 lands. To avoid that temporal coupling, **PR 0.4 introduces both `imageRepo` and `IDProvider` together** and the service tests set `ids: image.NewUUIDProvider()` (or a `*CounterIDProvider`) on every literal. PR 0.7 then has only one job: wire the providers into `BootApp`.

- [ ] **Step 1a: Add the `imageRepo` test seam to `image/service.go`.**

Open `api/internal/image/service.go` and add **above** the `Service` struct:

```go
// imageRepo is a narrow seam for service-level tests. *Repo satisfies it; this
// interface is not exported.
//
// IMPORTANT: signatures must match repo.go exactly. Insert returns
// (*InsertResult, error) — pointer to InsertResult — and FindByClientImageID
// returns nil (not the zero value) when the row is missing.
type imageRepo interface {
	FindByClientImageID(ctx context.Context, artworkID, clientImageID string) (*InsertedImage, error)
	Insert(ctx context.Context, in InsertInput) (*InsertResult, error)
}
```

- [ ] **Step 1b: Add the `IDProvider` interface and providers in `api/internal/image/idprovider.go`.**

Write `api/internal/image/idprovider.go`:

```go
// api/internal/image/idprovider.go
package image

import (
	"fmt"

	"github.com/google/uuid"
)

// IDProvider yields per-row IDs for image uploads. Production uses
// uuidIDProvider (a thin wrapper over uuid.NewString); the contract suite
// injects CounterIDProvider so snapshot bytes are stable.
type IDProvider interface {
	NewID() string
}

type uuidIDProvider struct{}

func (uuidIDProvider) NewID() string { return uuid.NewString() }

// NewUUIDProvider returns the production IDProvider.
func NewUUIDProvider() IDProvider { return uuidIDProvider{} }

// CounterIDProvider is a deterministic IDProvider for tests. The Nth call
// returns "00000000-0000-0000-0000-NNNNNNNNNNNN" (12-digit zero-padded N).
type CounterIDProvider struct {
	N int
}

func (c *CounterIDProvider) NewID() string {
	c.N++
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", c.N)
}
```

- [ ] **Step 1c: Wire `ids` into `Service`.**

Edit `api/internal/image/service.go` to add the field, default it in `NewService`, expose a test constructor, and use it at the upload site:

```
type Service struct {
    store    storage.Storage
    images   imageRepo
    artworks *artwork.Repo
+   ids      IDProvider
}

- func NewService(s storage.Storage, im *Repo, a *artwork.Repo) *Service {
- 	return &Service{store: s, images: im, artworks: a}
- }
+ func NewService(s storage.Storage, im *Repo, a *artwork.Repo) *Service {
+ 	return &Service{store: s, images: im, artworks: a, ids: NewUUIDProvider()}
+ }
+
+ // NewServiceWithIDs is the test-mode constructor. The contract suite passes a
+ // deterministic *CounterIDProvider here. Production uses NewService.
+ func NewServiceWithIDs(s storage.Storage, im *Repo, a *artwork.Repo, ids IDProvider) *Service {
+ 	return &Service{store: s, images: im, artworks: a, ids: ids}
+ }
```

In the same file at line 84, replace:

```
- 	imgID := uuid.NewString()
+ 	imgID := s.ids.NewID()
```

The `github.com/google/uuid` import in service.go can stay (still used by `idprovider.go` via the `image` package; `service.go` itself no longer references it). Run `go vet ./...` after — if vet flags the unused import in `service.go`, remove it.

Then change the field type:

```
- type Service struct {
- 	store    storage.Storage
- 	images   *Repo
- 	artworks *artwork.Repo
- }
+ type Service struct {
+ 	store    storage.Storage
+ 	images   imageRepo
+ 	artworks *artwork.Repo
+ }
```

`NewService`'s signature stays the same — `*Repo` satisfies `imageRepo`.

- [ ] **Step 2: Verify the build still passes and existing image tests still go green.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go build ./...
go test ./internal/image/... -count=1
```

Expected: green. No callers change because `*Repo` satisfies the new `imageRepo` interface, and `NewService` defaults `ids` so existing tests are untouched.

- [ ] **Step 3: Add the focused service tests.**

Write `api/internal/image/service_test.go`:

```go
package image

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"local/art-web/api/internal/artwork"
)

type stubRepo struct {
	findFn   func(ctx context.Context, art, cid string) (*InsertedImage, error)
	insertFn func(ctx context.Context, in InsertInput) (*InsertResult, error)
}

func (s *stubRepo) FindByClientImageID(ctx context.Context, art, cid string) (*InsertedImage, error) {
	return s.findFn(ctx, art, cid)
}
func (s *stubRepo) Insert(ctx context.Context, in InsertInput) (*InsertResult, error) {
	return s.insertFn(ctx, in)
}

type stubStore struct {
	puts    []string
	deletes []string
	failPut bool
}

func (s *stubStore) Put(_ context.Context, k string, _ io.Reader, _ string) error {
	if s.failPut {
		return errors.New("storage Put failed")
	}
	s.puts = append(s.puts, k)
	return nil
}
func (s *stubStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *stubStore) Delete(_ context.Context, k string) error {
	s.deletes = append(s.deletes, k)
	return nil
}
func (s *stubStore) Move(context.Context, string, string) error { return nil }
func (s *stubStore) Exists(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubStore) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func loadJPEG(t *testing.T) []byte {
	raw, err := os.ReadFile("testdata/sample.jpg")
	if err != nil {
		t.Fatalf("read sample.jpg: %v (cd into image/ to make testdata available)", err)
	}
	return raw
}

func TestUploadOne_StoragePutFailure_Surfaces(t *testing.T) {
	repo := &stubRepo{
		findFn:   func(_ context.Context, _, _ string) (*InsertedImage, error) { return nil, nil },
		insertFn: func(_ context.Context, _ InsertInput) (*InsertResult, error) { return &InsertResult{}, nil },
	}
	store := &stubStore{failPut: true}
	// ids must be non-nil — UploadOne calls s.ids.NewID() at line 84 even
	// when Put fails before reaching the insert (the call sequence is decode,
	// then ids.NewID, then Put). NewUUIDProvider is fine because the failing
	// Put short-circuits before we'd otherwise compare IDs across runs.
	svc := &Service{store: store, images: repo, artworks: nil, ids: NewUUIDProvider()}
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}

	_, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err == nil {
		t.Fatal("expected storage Put error to surface")
	}
	if !strings.Contains(err.Error(), "storage Put failed") {
		t.Fatalf("expected wrapped storage error, got %v", err)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("Delete must NOT fire on Put failure (orphan only on Insert failure); got deletes=%v", store.deletes)
	}
}

func TestUploadOne_InsertFails_OrphanIsDeleted(t *testing.T) {
	repo := &stubRepo{
		findFn: func(_ context.Context, _, _ string) (*InsertedImage, error) { return nil, nil },
		insertFn: func(_ context.Context, _ InsertInput) (*InsertResult, error) {
			return nil, errors.New("insert failure")
		},
	}
	store := &stubStore{}
	svc := &Service{store: store, images: repo, artworks: nil, ids: NewUUIDProvider()}
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}

	_, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err == nil {
		t.Fatal("expected insert error to surface")
	}
	if len(store.deletes) != 1 || store.deletes[0] == "" {
		t.Fatalf("expected one Delete for orphan cleanup, got deletes=%v", store.deletes)
	}
	if store.deletes[0] != store.puts[0] {
		t.Fatalf("orphan Delete key %q must equal Put key %q", store.deletes[0], store.puts[0])
	}
}

func TestUploadOne_IdempotentInsertButRowNotFound_ErrorsClearly(t *testing.T) {
	calls := 0
	repo := &stubRepo{
		findFn: func(_ context.Context, _, _ string) (*InsertedImage, error) {
			calls++
			// First call (pre-Insert): no row → keep going.
			// Second call (after idempotent Insert): also nil → triggers the error
			// at service.go line 109-111.
			return nil, nil
		},
		insertFn: func(_ context.Context, _ InsertInput) (*InsertResult, error) {
			return &InsertResult{ID: "imgX", Existed: true}, nil
		},
	}
	store := &stubStore{}
	svc := &Service{store: store, images: repo, artworks: nil, ids: NewUUIDProvider()}
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}

	_, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err == nil || !strings.Contains(err.Error(), "idempotent insert") {
		t.Fatalf("expected idempotent-insert sentinel error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 FindByClientImageID calls, got %d", calls)
	}
}
```

- [ ] **Step 4: Run.**

Run: `go test ./internal/image/... -run 'TestUploadOne_' -v`
Expected: PASS.

- [ ] **Step 5: Verify slice coverage.**

```bash
go test -cover ./internal/image/...
```

Expected: `coverage: NN.N% of statements` ≥ 80.0.

- [ ] **Step 6: Commit.**

```bash
git add api/internal/image/service.go api/internal/image/idprovider.go api/internal/image/service_test.go
git commit -m "[0.4-image] feat: add IDProvider + imageRepo seam; cover failure paths"
```

### Task 4.3 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.4-image
gh pr create --base master --title "[0.4-image] Lift image package coverage to ≥ 80%" --body "$(cat <<'EOF'
## Summary
- Add narrow `imageRepo` interface seam in service.go (no behavior change).
- Add `IDProvider` interface + `NewUUIDProvider` (production) + `CounterIDProvider` (deterministic for contract suite). Switch `service.go:84` from `uuid.NewString()` to `s.ids.NewID()`. PR 0.7 then wires `CounterIDProvider` into `BootApp`.
- Cover storage Put failure (no spurious Delete), Insert failure (orphan cleanup fires), idempotent-insert-but-row-missing race-window error.

## Test plan
- [ ] `go test -cover ./internal/image/...` ≥ 80 %
- [ ] All existing tests still pass

Refs spec §8.1 PR 0.4.
EOF
)"
```

---

## PR 0.5 — `httpapi` 69.2 → 80 %

**Branch:** `phase0/0.5-httpapi`

**Goal:** Cover the privacy matrix completeness, devseed gated routes (coverage only — `/dev/seed` is excluded from contract per spec §5.2.1), and one test per error code listed in spec §3 — including 412 / 415 / 422 / 502.

**Acceptance criteria:** `go test -cover ./internal/httpapi/...` ≥ 80 %.

### Task 5.1 — Identify uncovered lines

- [ ] **Step 1: Generate a coverage report.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go test -coverprofile=/tmp/httpapi.out ./internal/httpapi/...
go tool cover -func=/tmp/httpapi.out | grep -v "100.0%"
```

Cross-reference the output with spec §3's error-code list. Any code that doesn't appear here in a hit ≥ 1 needs a focused test.

### Task 5.2 — Cover the privacy-matrix gaps

**Files:**
- Modify: `api/internal/httpapi/privacy_matrix_test.go`

- [ ] **Step 1: Read the existing matrix to find which (viewer × resource × action) cells are missing.**

```bash
grep -E '^func Test' /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/httpapi/privacy_matrix_test.go
```

The complete matrix per spec §5.2 is:

| Viewer | Resource | GET /artworks/{id} | PATCH /artworks/{id} | DELETE /artworks/{id} | POST /artworks/{id}/images |
|---|---|---|---|---|---|
| owner  | public  | 200 | 204 | 204 | 200/201 |
| owner  | private | 200 | 204 | 204 | 200/201 |
| other  | public  | 200 | 404 | 404 | 404 |
| other  | private | 404 | 404 | 404 | 404 |
| anon   | public  | 200 | 401 | 401 | 401 |
| anon   | private | 404 | 401 | 401 | 401 |

For each missing cell, add a `t.Run(...)` subtest using `env.request(...)` from `httpapi/testutil_test.go`.

- [ ] **Step 2: Append the missing tests in a new function `TestPrivacyMatrix_Gaps`.**

Add to `api/internal/httpapi/privacy_matrix_test.go`:

```go
func TestPrivacyMatrix_Gaps(t *testing.T) {
	env := setupMatrixEnv(t)

	cases := []struct {
		name   string
		viewer string
		method string
		path   string
		want   int
	}{
		// other → private GET = 404 (not 403, per spec §5.2.0a)
		{"other_get_private", "other", "GET", "/artworks/" + env.QID, 404},
		// anon → private GET = 404
		{"anon_get_private", "anon", "GET", "/artworks/" + env.QID, 404},
		// other → public PATCH = 404
		{"other_patch_public", "other", "PATCH", "/artworks/" + env.PID, 404},
		// other → public DELETE = 404
		{"other_delete_public", "other", "DELETE", "/artworks/" + env.PID, 404},
		// anon → public PATCH = 401
		{"anon_patch_public", "anon", "PATCH", "/artworks/" + env.PID, 401},
		// anon → public DELETE = 401
		{"anon_delete_public", "anon", "DELETE", "/artworks/" + env.PID, 401},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, code := env.request(t, tc.viewer, tc.method, tc.path)
			if code != tc.want {
				t.Fatalf("status=%d want %d", code, tc.want)
			}
		})
	}
}
```

> The PATCH cases pass an empty body. The current handler reads the body via `json.NewDecoder` and returns `bad_json` (400) for empty input. To produce 401 / 404 we need to short-circuit before body parsing — verify by reading `httpapi/artworks.go:170-188`. Auth middleware fires before the handler, so anon → 401 is correct. The owner-check fires after JSON decode, so other → 404 requires a syntactically-valid PATCH body. **If a subtest fails because of bad-JSON 400, change the test to send a minimal valid body** (e.g., `{"title":"x"}`) using a new request helper variant. Add this:

```go
// requestWithJSONBody is the variant for PATCH cases.
func (env *MatrixEnv) requestWithJSONBody(t *testing.T, viewer, method, path, body string) (string, int) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	switch viewer {
	case "owner":
		req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	case "other":
		req.AddCookie(&http.Cookie{Name: "auth", Value: env.otherToken})
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Body)
	return string(b), rec.Code
}
```

(Add `"strings"` import if not present.)

Then PATCH subtests use:
```go
_, code := env.requestWithJSONBody(t, tc.viewer, tc.method, tc.path, `{"title":"x"}`)
```

- [ ] **Step 3: Run.**

Run: `go test ./internal/httpapi/... -run TestPrivacyMatrix_Gaps -v`
Expected: PASS for all subtests.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/httpapi/privacy_matrix_test.go api/internal/httpapi/testutil_test.go
git commit -m "[0.5-httpapi] test: fill privacy-matrix gaps (other/anon × public/private × verbs)"
```

### Task 5.3 — Cover every Shape-A and Shape-B error code via PR-0.5 + the existing tests

**Files:**
- Create: `api/internal/httpapi/error_codes_test.go`

The contract has many error codes (spec §3 lists ~25). Some are already covered by existing tests in lower-level packages (`internal/image/handler_test.go`, `internal/auth/handlers_test.go`), and PR 0.7's matrix will then exercise them again from the HTTP boundary. This task closes the **httpapi-package** gap so the slice's coverage hits ≥ 80 %, and also pins the response body shape (status + JSON keys) for codes that lower-level tests don't pin.

#### Coverage mapping — every code listed in spec §3

The table below enumerates every Shape-A and Shape-B error code from spec §3 and identifies where each is exercised. PR 0.5 adds tests only for cells marked `add (PR 0.5)` so we don't duplicate work.

| Status | Code | Shape | Source endpoint(s) | Where covered |
|---|---|---|---|---|
| 401 | `unauthorized` | A | `/me`, auth-gated PATCH/DELETE/POST artworks | privacy-matrix tests + `add (PR 0.5)` for /me |
| 404 | `not_found` | A | GET/PATCH/DELETE artwork by id, GET user, GET tag, image upload (non-owner) | privacy-matrix tests (already) |
| 400 | `bad_json` | A | POST/PATCH artworks (decode) | `add (PR 0.5)` |
| 400 | `bad_visibility` | A | POST/PATCH artworks | `add (PR 0.5)` |
| 400 | `bad_cover_position` | A | PATCH artworks | `add (PR 0.5)` |
| 400 | `bad_cursor` | **B** | GET /artworks?cursor=…, GET /users/{slug}?cursor=… | `add (PR 0.5)` |
| 500 | `create_failed` | A | POST /artworks | covered transitively in matrix; not actionable from httpapi tests (requires DB-side failure) — note in PR description |
| 500 | `patch_failed` | A | PATCH /artworks | same — DB-failure path |
| 500 | `flip_failed` | A | PATCH visibility | same |
| 500 | `tag_failed` | A | tags via PATCH | same |
| 500 | `delete_failed` | A | DELETE /artworks | same |
| 500 | `list_failed` | A | GET /artworks, GET /users/{slug} | same |
| 500 | `user_lookup_failed` | A | /me when DB fails | same |
| 500 | `user_failed` | A | GET /artworks/{id} when artist lookup fails | same |
| 404 | `unknown_provider` | A | /auth/{provider}/start, /auth/{provider}/callback | `add (PR 0.5)` |
| 400 | `bad_state` | A | /auth/{provider}/callback | covered in `auth/handlers_test.go` (verify with grep — if absent, `add (PR 0.5)`) |
| 502 | `exchange_failed` | A | /auth/{provider}/callback | covered in `auth/handlers_test.go` (verify) |
| 500 | `upsert_failed` | A | /auth/{provider}/callback | covered in `auth/handlers_test.go` (verify) |
| 500 | `sign_failed` | A | /auth/{provider}/callback | covered in `auth/handlers_test.go` (verify) |
| 415 | `unsupported_media_type` | A | POST /artworks/{id}/images | covered in `image/handler_test.go` + `add (PR 0.5)` to pin the body shape |
| 400 | `bad_multipart` | A | POST /artworks/{id}/images | `add (PR 0.5)` |
| 400 | `manifest_required` | A | POST /artworks/{id}/images | `add (PR 0.5)` |
| 400 | `file_count_mismatch` | A | POST /artworks/{id}/images | `add (PR 0.5)` |
| 400 | `open_file` | A | POST /artworks/{id}/images | very hard to provoke (requires multipart file Open() error); document as "captured by PR 0.7 matrix only if it triggers organically; otherwise document the gap." |
| 412 | `position_taken` | A | POST /artworks/{id}/images | covered in `image/handler_test.go` |
| 422 | `too_large` | A | POST /artworks/{id}/images | `add (PR 0.5)` |
| 422 | `decode_failed` | A | POST /artworks/{id}/images | covered in `image/handler_test.go` |
| 500 | `upload_failed` | A | POST /artworks/{id}/images | DB-failure path (skip) |
| 409 | `fingerprint_mismatch` | **B** | POST /artworks/{id}/images | covered in `image/handler_test.go` |
| 422 | `content_type_mismatch` | **B** | POST /artworks/{id}/images | covered in `image/handler_test.go` |
| 400 | (free-form ParseManifest message) | A-quirky | POST /artworks/{id}/images | `add (PR 0.5)` — pin the quirky shape |

**Cells marked "DB-failure path (skip)"**: produced only when underlying DB operations fail at runtime. Reproducing them requires either a stubbed repo (which contradicts the integration-test approach) or fault injection in Postgres (brittle). Spec §3 says these codes are "frozen into the contract" — they'll be locked from snapshots PR 0.8 captures *if* organic test traffic hits them. Document as known gaps in the PR description.

#### Test snippets

The tests below cover every cell marked `add (PR 0.5)`. They're deliberately small and assert the exact JSON shape so PR 0.7 / 0.8 can lock the bytes.

Write `api/internal/httpapi/error_codes_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorBody is the universal Shape-A/B decoder.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func decodeError(t *testing.T, body io.Reader) errorBody {
	t.Helper()
	var b errorBody
	if err := json.NewDecoder(body).Decode(&b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return b
}

// ---- /artworks ----

func TestErrors_ArtworksList_BadCursor(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/artworks?cursor=not-base64!!")
	if code != 400 {
		t.Fatalf("status=%d want 400; body=%s", code, body)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_cursor" {
		t.Fatalf("error=%q want bad_cursor", got.Error)
	}
	if got.Message == "" {
		t.Fatal("Shape B requires non-empty message")
	}
}

func TestErrors_ArtworkPatch_BadJSON(t *testing.T) {
	env := setupMatrixEnv(t)
	_, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, "not valid json")
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
}

func TestErrors_ArtworkPatch_BadVisibility(t *testing.T) {
	env := setupMatrixEnv(t)
	_, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, `{"visibility":"draft"}`)
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
}

func TestErrors_ArtworkPatch_BadCoverPosition(t *testing.T) {
	env := setupMatrixEnv(t)
	_, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, `{"cover_position":-1}`)
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
}

func TestErrors_ArtworkCreate_BadVisibility(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.requestWithJSONBody(t, "owner", "POST", "/artworks", `{"title":"x","visibility":"draft"}`)
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_visibility" {
		t.Fatalf("error=%q want bad_visibility", got.Error)
	}
}

// ---- /auth ----

func TestErrors_Auth_UnknownProvider(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/auth/notreal/start")
	if code != 404 {
		t.Fatalf("status=%d want 404", code)
	}
	if !strings.Contains(body, "unknown_provider") {
		t.Fatalf("expected unknown_provider, got %s", body)
	}
}

// ---- POST /artworks/{id}/images — 415, 412, 422 ----

func TestErrors_ImageUpload_415_NonMultipart(t *testing.T) {
	env := setupMatrixEnv(t)

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 415 {
		t.Fatalf("status=%d want 415", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "unsupported_media_type" {
		t.Fatalf("error=%q want unsupported_media_type", got.Error)
	}
}

func TestErrors_ImageUpload_400_ManifestRequired(t *testing.T) {
	env := setupMatrixEnv(t)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	// no manifest field, just an empty multipart envelope
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "manifest_required" {
		t.Fatalf("error=%q want manifest_required", got.Error)
	}
}

func TestErrors_ImageUpload_400_FileCountMismatch(t *testing.T) {
	env := setupMatrixEnv(t)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("manifest", `[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"}]`)
	// no `files` field — count mismatch (manifest=1, files=0)
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "file_count_mismatch" {
		t.Fatalf("error=%q want file_count_mismatch", got.Error)
	}
}

// 412 / 422-decode / 409-fingerprint / 422-content_type / 412-position
// image-upload paths are already covered by internal/image/handler_test.go.
// The privacy-matrix and these tests cover the remaining httpapi-package surface
// for those statuses; no duplicate image tests belong here.

// ---- Additional cells from the coverage table ----

// /me — unauthorized 401 (Shape A).
func TestErrors_Me_Unauthorized(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/me")
	if code != 401 {
		t.Fatalf("status=%d want 401", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "unauthorized" {
		t.Fatalf("error=%q want unauthorized", got.Error)
	}
}

// PATCH bad_json (decode failure).
func TestErrors_ArtworkPatch_BadJSON_Body(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, "this is not json")
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_json" {
		t.Fatalf("error=%q want bad_json", got.Error)
	}
}

// POST bad_json (decode failure).
func TestErrors_ArtworkCreate_BadJSON(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.requestWithJSONBody(t, "owner", "POST", "/artworks", "not json")
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_json" {
		t.Fatalf("error=%q want bad_json", got.Error)
	}
}

// /users/{slug} cursor parse failure (Shape B).
func TestErrors_UserProfile_BadCursor(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/users/alice-"+env.AliceSuffix+"?cursor=not-base64!!")
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_cursor" || got.Message == "" {
		t.Fatalf("expected Shape B bad_cursor, got %+v", got)
	}
}

// /auth/{provider}/callback unknown_provider (404).
func TestErrors_AuthCallback_UnknownProvider(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/auth/notreal/callback?state=x&code=y")
	if code != 404 {
		t.Fatalf("status=%d want 404", code)
	}
	if !strings.Contains(body, "unknown_provider") {
		t.Fatalf("expected unknown_provider, got %s", body)
	}
}

// 400 bad_multipart — declare multipart but send a malformed body.
func TestErrors_ImageUpload_400_BadMultipart(t *testing.T) {
	env := setupMatrixEnv(t)

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images",
		strings.NewReader("not a real multipart payload"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=fake")
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "bad_multipart" {
		t.Fatalf("error=%q want bad_multipart", got.Error)
	}
}

// 422 too_large — manifest declares a file size > 25 MB.
func TestErrors_ImageUpload_422_TooLarge(t *testing.T) {
	env := setupMatrixEnv(t)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("manifest",
		`[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"}]`)
	w, _ := mw.CreateFormFile("files", "f.jpg")
	// 26 MB — exceeds image.MaxBytes (25 MB).
	_, _ = w.Write(bytes.Repeat([]byte{0xFF}, 26*1024*1024))
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 422 {
		t.Fatalf("status=%d want 422", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "too_large" {
		t.Fatalf("error=%q want too_large", got.Error)
	}
}

// Shape-A quirky — ParseManifest emits the parse-error string verbatim as the
// "error" value. Pin this so PR 0.8 captures the quirky shape and Phase 1's
// translator faithfully reproduces it.
func TestErrors_ImageUpload_400_ParseManifestQuirk(t *testing.T) {
	env := setupMatrixEnv(t)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	// Manifest with an invalid line that ParseManifest will reject. The exact
	// message string is implementation-defined; we only assert it's NOT a stable
	// snake_case code (i.e., it contains "manifest" or whitespace).
	_ = mw.WriteField("manifest", "this is not a manifest")
	w, _ := mw.CreateFormFile("files", "f.jpg")
	_, _ = w.Write([]byte("anything"))
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	got := decodeError(t, rec.Body)
	// The quirk: the value is the parse-error string itself, not a snake_case
	// code. We don't assert the exact message — that's what PR 0.8 locks. We
	// do assert it's NOT one of the stable codes.
	stableCodes := map[string]bool{
		"bad_json": true, "bad_multipart": true, "manifest_required": true,
		"file_count_mismatch": true, "unsupported_media_type": true,
	}
	if stableCodes[got.Error] {
		t.Fatalf("expected free-form ParseManifest message, got stable code %q", got.Error)
	}
	if got.Message != "" {
		t.Fatalf("ParseManifest quirk has no `message` key; got %q", got.Message)
	}
}
```

> Add `"net/http"`, `"bytes"`, `"mime/multipart"`, `"net/http/httptest"` to imports if not already present.

- [ ] **Step 2: Run.**

Run: `go test ./internal/httpapi/... -run 'TestErrors_' -v`
Expected: PASS for all.

- [ ] **Step 3: Commit.**

```bash
git add api/internal/httpapi/error_codes_test.go
git commit -m "[0.5-httpapi] test: assert exact body shape for every Shape-A/B error code"
```

### Task 5.4 — Cover the devseed gating

**Files:**
- Modify: `api/internal/httpapi/devseed_test.go`

- [ ] **Step 1: Read the existing devseed test to see what's already covered.**

```bash
sed -n '1,50p' /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/httpapi/devseed_test.go
```

- [ ] **Step 2: If the `AppEnv != "test"` 404 case is missing, append:**

```go
func TestDevSeed_NotMounted_When_AppEnv_NotTest(t *testing.T) {
	deps := testDeps(t, "production")        // helper from testutil_test.go
	r := httpapi.New(deps)

	req := httptest.NewRequest("POST", "/dev/seed", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("expected /dev/seed to be unmounted in non-test env; status=%d", rec.Code)
	}
}
```

- [ ] **Step 3: Run, then verify slice coverage.**

```bash
go test ./internal/httpapi/... -run TestDevSeed_NotMounted -v
go test -cover ./internal/httpapi/...
```

Expected: PASS, `coverage: NN.N% of statements` ≥ 80.0. If not, generate an HTML coverage report (`go tool cover -html=/tmp/httpapi.out`) and add a focused test for the highlighted branch.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/httpapi/devseed_test.go
git commit -m "[0.5-httpapi] test: assert /dev/seed 404s outside AppEnv=test"
```

### Task 5.5 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.5-httpapi
gh pr create --base master --title "[0.5-httpapi] Lift httpapi package coverage to ≥ 80%" --body "$(cat <<'EOF'
## Summary
- Fill privacy-matrix gaps (other/anon × public/private × {GET,PATCH,DELETE}).
- Add one test per Shape-A and Shape-B error code from spec §3.
- Assert /dev/seed 404 in non-test env.

## Test plan
- [ ] `go test -cover ./internal/httpapi/...` ≥ 80 %
- [ ] No new behavior; pure test additions

Refs spec §8.1 PR 0.5.
EOF
)"
```

---

## PR 0.6 — `db` + `artwork` top-up

**Branch:** `phase0/0.6-db-artwork-topup`

**Goal:** `artwork` 78.3 → 80 (visibility transition edge cases — required for slice gate). `db` 75 → 80 best-effort (excluded from the gate per spec §5.4 but worth doing while harness work is fresh).

**Acceptance criteria:**
- `go test -cover ./internal/artwork/...` ≥ 80 %.
- `go test -cover ./internal/db/...` ≥ 75 % (no regression).

### Task 6.1 — Cover the artwork visibility-flip edge cases

**Files:**
- Modify: `api/internal/artwork/visibility_test.go`

- [ ] **Step 1: Identify the uncovered branches in `visibility.go`.**

The flip flow has these failure modes that need explicit tests (verified against `visibility.go`):

1. `target` is neither `"public"` nor `"private"` → `errBadTarget`. (Likely already covered.)
2. `Pool().Query` fails. (Hard to provoke; skip unless coverage demands.)
3. **`storage.Move` fails on the forward direction → `rollbackMoves` on completed prefix.** (Likely missing.)
4. **`storage.Move` fails on the reverse direction → `RollbackLog` fires.** (Likely missing.)
5. **Tx Begin / Exec / Commit failure → forward moves are rolled back.** (Likely missing for at least one of the three.)

- [ ] **Step 2: Read the existing test to find which gaps are real.**

```bash
grep -E '^func Test' /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/artwork/visibility_test.go
```

- [ ] **Step 3: Add the missing tests.**

The store can be substituted with a test double because `*VisibilityService` accepts the `storage.Storage` interface. Append:

```go
type seqStore struct {
	storage.Storage
	moveOrder []string
	failOnDst string // when Move(src,dst) sees this dst, return error
}

func (s *seqStore) Move(ctx context.Context, src, dst string) error {
	s.moveOrder = append(s.moveOrder, dst)
	if dst == s.failOnDst {
		return fmt.Errorf("simulated move failure to %s", dst)
	}
	return s.Storage.Move(ctx, src, dst)
}

func TestFlip_StorageMoveFailure_RollsBackForwardMoves(t *testing.T) {
	pool := newPool(t)             // existing helper in this file
	defer pool.Close()
	repo := artwork.NewRepo(pool)

	// Seed: artwork with 3 images at private/{art}/{i}.jpg.
	uid := seedUser(t, pool)
	aid := seedArtworkWithImages(t, pool, uid, "private", 3)

	store := &seqStore{Storage: storage.NewLocalFS(t.TempDir())}
	// Make all three private files exist.
	for i := 0; i < 3; i++ {
		_ = store.Storage.Put(t.Context(), fmt.Sprintf("private/%s/%d.jpg", aid, i),
			strings.NewReader("x"), "image/jpeg")
	}
	// Fail on the SECOND forward move (public/<aid>/1.jpg).
	store.failOnDst = "public/" + aid + "/1.jpg"

	svc := artwork.NewVisibilityService(repo, store)
	err := svc.Flip(t.Context(), aid, "public")
	if err == nil {
		t.Fatal("expected flip to fail when storage.Move fails on the second image")
	}
	// First completed forward move should have been rolled back: the file
	// should NOT exist at public/.../0.jpg, and SHOULD exist at private/.../0.jpg.
	pubExists, _ := store.Storage.Exists(t.Context(), fmt.Sprintf("public/%s/0.jpg", aid))
	privExists, _ := store.Storage.Exists(t.Context(), fmt.Sprintf("private/%s/0.jpg", aid))
	if pubExists {
		t.Fatal("public/0.jpg should not exist after rollback")
	}
	if !privExists {
		t.Fatal("private/0.jpg should exist after rollback")
	}
}

func TestFlip_RollbackLogFires_WhenUndoMoveFails(t *testing.T) {
	pool := newPool(t)
	defer pool.Close()
	repo := artwork.NewRepo(pool)

	uid := seedUser(t, pool)
	aid := seedArtworkWithImages(t, pool, uid, "private", 2)

	// Two-direction failing store: forward succeeds, reverse fails. After the
	// second forward Move triggers the simulated failure path, the reverse
	// rollback for the first forward move will also fail → RollbackLog fires.
	base := storage.NewLocalFS(t.TempDir())
	for i := 0; i < 2; i++ {
		_ = base.Put(t.Context(), fmt.Sprintf("private/%s/%d.jpg", aid, i),
			strings.NewReader("x"), "image/jpeg")
	}
	store := &reversingFailStore{Storage: base, aid: aid, failPathDst: "public/" + aid + "/1.jpg"}

	var rollbackCalls int
	prev := artwork.RollbackLog
	artwork.RollbackLog = func(err error) { rollbackCalls++ }
	defer func() { artwork.RollbackLog = prev }()

	svc := artwork.NewVisibilityService(repo, store)
	if err := svc.Flip(t.Context(), aid, "public"); err == nil {
		t.Fatal("expected flip to fail")
	}
	if rollbackCalls == 0 {
		t.Fatal("expected RollbackLog to fire when undo move fails")
	}
}

// reversingFailStore succeeds on forward moves (private→public) and fails on
// reverse moves (public→private), simulating a one-way storage outage.
type reversingFailStore struct {
	storage.Storage
	aid         string
	failPathDst string
}

func (s *reversingFailStore) Move(ctx context.Context, src, dst string) error {
	// Treat the "trigger" forward move as the failure that initiates rollback.
	if dst == s.failPathDst {
		return fmt.Errorf("trigger forward move failed: %s→%s", src, dst)
	}
	// Reverse direction (public→private): fail.
	if strings.HasPrefix(src, "public/"+s.aid+"/") && strings.HasPrefix(dst, "private/"+s.aid+"/") {
		return fmt.Errorf("simulated reverse-move failure: %s→%s", src, dst)
	}
	return s.Storage.Move(ctx, src, dst)
}
```

> The helpers `newPool`, `seedUser`, and `seedArtworkWithImages` may not exist yet. Read the file to confirm. If `newPool` is called something else (e.g., `setupRepo`), adapt accordingly. If `seedArtworkWithImages` doesn't exist, write a small helper at the top of the file that calls `repo.Create` then runs `pool.Exec(ctx, INSERT INTO artwork_images ...)` directly N times.

- [ ] **Step 4: Run.**

Run: `go test ./internal/artwork/... -run 'TestFlip_StorageMoveFailure|TestFlip_RollbackLogFires' -v`
Expected: PASS.

- [ ] **Step 5: Verify slice coverage.**

```bash
go test -cover ./internal/artwork/...
```

Expected: `coverage: NN.N% of statements` ≥ 80.0.

- [ ] **Step 6: Commit.**

```bash
git add api/internal/artwork/visibility_test.go
git commit -m "[0.6-db-artwork-topup] test: cover visibility-flip rollback paths"
```

### Task 6.2 — Best-effort `db` top-up

**Files:**
- Modify: `api/internal/db/pool_test.go`

- [ ] **Step 1: Find uncovered branches.**

```bash
go test -coverprofile=/tmp/db.out ./internal/db/...
go tool cover -func=/tmp/db.out | grep -v "100.0%"
```

- [ ] **Step 2: Add a focused bad-DSN test.**

Append to `api/internal/db/pool_test.go`:

```go
func TestNew_BadDSN_ReturnsError(t *testing.T) {
	_, err := db.New(context.Background(), "this-is-not-a-valid-dsn")
	if err == nil {
		t.Fatal("expected error for malformed DSN")
	}
}
```

- [ ] **Step 3: Run, then check coverage.**

```bash
go test ./internal/db/... -run TestNew_BadDSN -v
go test -cover ./internal/db/...
```

If coverage is still below 80 %, the remaining uncovered code is likely the long-running pool-Close path; document the gap in the PR description rather than forcing a flaky test.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/db/pool_test.go
git commit -m "[0.6-db-artwork-topup] test: cover db.New bad-DSN error path"
```

### Task 6.3 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.6-db-artwork-topup
gh pr create --base master --title "[0.6-db-artwork-topup] artwork to ≥ 80% + db top-up" --body "$(cat <<'EOF'
## Summary
- Cover visibility-flip storage-Move rollback (forward failure rolls back completed prefix).
- Cover RollbackLog firing when the undo move itself fails.
- Best-effort: cover db.New bad-DSN.

## Test plan
- [ ] `go test -cover ./internal/artwork/...` ≥ 80 %
- [ ] `go test -cover ./internal/db/...` no regression

Refs spec §8.1 PR 0.6.
EOF
)"
```

---

## PR 0.7 — Build the HTTP contract matrix + deterministic injection

**Branch:** `phase0/0.7-contract-matrix`

**Goal:** Per spec §8.1: add the cartesian-matrix test file, switch the two Go-side dynamic sources to injectable interfaces (no behavior change), wire the normalizer, and write the meta-test that scans goldens for forbidden statuses. **No goldens yet** — tests are expected to fail until PR 0.8 captures them.

**Acceptance criteria:**
- `IDProvider` was already introduced in PR 0.4; PR 0.7 only wires it into BootApp.
- `RandReader` interface added; production passes `crypto/rand.Reader`; `auth/handlers.go:randState` uses it.
- `/dev/seed` accepts a `?suffix=` query parameter for deterministic slug suffixes.
- Cartesian matrix file exists under `internal/httpapi/contract/` and enumerates every (auth × resource × shape) cell from spec §5.2 — including the auth-callback 302 (success) and 502 (exchange_failed) branches.
- Forbidden-status meta-test file exists.
- `go build ./...` passes; existing tests unchanged.

### Task 7.1 — `IDProvider` already exists from PR 0.4

`IDProvider`, `NewUUIDProvider`, `CounterIDProvider`, and `NewServiceWithIDs` were introduced in PR 0.4 (Task 4.2 Step 1b/1c) so the new service tests could initialize `ids` correctly. PR 0.7 has no work to do here beyond using these in `BootApp` (Task 7.3) and the matrix (Task 7.4). **No code changes in this task.**

- [ ] **Step 1: Confirm the providers are already on master.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
test -f internal/image/idprovider.go && echo OK || echo "missing — PR 0.4 didn't ship"
grep -q "func NewServiceWithIDs" internal/image/service.go && echo OK || echo "missing"
grep -q "s.ids.NewID()" internal/image/service.go && echo OK || echo "missing"
```

Expected: three `OK` lines. If any is missing, stop — PR 0.4 didn't land cleanly and Phase 0 ordering was broken.

### Task 7.2 — Add `RandReader` (no-op refactor)

**Files:**
- Create: `api/internal/auth/randreader.go`
- Modify: `api/internal/auth/handlers.go`

- [ ] **Step 1: Create the interface.**

Write `api/internal/auth/randreader.go`:

```go
// api/internal/auth/randreader.go
package auth

import (
	"crypto/rand"
	"io"
)

// RandReader yields the random source for OAuth state. Production uses
// crypto/rand.Reader; tests inject a deterministic source so snapshot bytes
// are stable.
type RandReader interface {
	Read(p []byte) (n int, err error)
}

// defaultRand wraps crypto/rand.Reader so we don't expose the *os.File-typed
// reader directly to callers who expect an interface.
var defaultRand RandReader = randAdapter{}

type randAdapter struct{}

func (randAdapter) Read(p []byte) (int, error) { return rand.Read(p) }

// stateRand is package-level so test code can swap it under sync. Production
// startup never mutates it.
var stateRand RandReader = defaultRand

// SetStateRandForTest is exported only for use under -tags=integration to swap
// in a deterministic reader for snapshot capture. Restore the previous value
// in t.Cleanup.
func SetStateRandForTest(r RandReader) (restore func()) {
	prev := stateRand
	stateRand = r
	return func() { stateRand = prev }
}

// readState is the internal seam for randState; tests override stateRand.
func readState(p []byte) (int, error) {
	return stateRand.Read(p)
}

// Keep io.Reader symbol used so the import stays warm under refactors.
var _ io.Reader = (*randAdapter)(nil)
```

- [ ] **Step 2: Edit `randState` to use the seam.**

In `api/internal/auth/handlers.go`, replace the body of `randState`:

```
- func randState() string {
- 	b := make([]byte, 16)
- 	_, _ = rand.Read(b)
- 	return hex.EncodeToString(b)
- }
+ func randState() string {
+ 	b := make([]byte, 16)
+ 	_, _ = readState(b)
+ 	return hex.EncodeToString(b)
+ }
```

You can now drop the unused `crypto/rand` import from `handlers.go` (`go vet` will tell you).

- [ ] **Step 3: Build & run.**

```bash
go build ./...
go test ./internal/auth/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/auth/handlers.go api/internal/auth/randreader.go
git commit -m "[0.7-contract-matrix] feat: add RandReader seam for deterministic OAuth state"
```

### Task 7.2b — Add `?suffix=` query param to `/dev/seed`

**Files:**
- Modify: `api/internal/httpapi/devseed.go`

**Why:** The contract suite seeds Alice and Bob via `/dev/seed`, but the current handler generates a random hex suffix at line 73 (`devRandHex(4)`). The suffix appears in the user slug, which appears in `/users/{slug}` response bodies — the normalizer doesn't catch arbitrary slugs, so replays would diverge. Accepting an optional `?suffix=` makes the seed deterministic without changing default behavior.

- [ ] **Step 1: Edit `devseed.go` to read `?suffix=`.**

In `api/internal/httpapi/devseed.go`, replace:

```
- 	suffix := devRandHex(4)
+ 	suffix := r.URL.Query().Get("suffix")
+ 	if suffix == "" {
+ 		suffix = devRandHex(4)
+ 	}
```

(The replacement is on or near `devseed.go:73`. Verify by reading the surrounding lines.)

- [ ] **Step 2: Add a test asserting deterministic suffix behavior.**

Append to `api/internal/httpapi/devseed_test.go`:

```go
func TestDevSeed_FixedSuffix_ProducesDeterministicSlug(t *testing.T) {
	deps := testDeps(t, "test")    // helper from testutil_test.go
	r := httpapi.New(deps)

	req := httptest.NewRequest("POST", "/dev/seed?suffix=fixed1234", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		AliceSlug string `json:"aliceSlug"`
		BobSlug   string `json:"bobSlug"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AliceSlug != "alice-fixed1234" {
		t.Fatalf("alice slug=%q want alice-fixed1234", resp.AliceSlug)
	}
	if resp.BobSlug != "bob-fixed1234" {
		t.Fatalf("bob slug=%q want bob-fixed1234", resp.BobSlug)
	}
}
```

(Add `"encoding/json"` to imports if not present.)

- [ ] **Step 3: Run.**

Run: `go test ./internal/httpapi/... -run TestDevSeed_FixedSuffix -v`
Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/httpapi/devseed.go api/internal/httpapi/devseed_test.go
git commit -m "[0.7-contract-matrix] feat: accept ?suffix= on /dev/seed for deterministic seeding"
```

### Task 7.3 — Wire injection options into `BootApp`

**Files:**
- Modify: `api/internal/dbtest/bootapp.go`

- [ ] **Step 1: Extend `BootOpts`.**

Add to the struct (alphabetical order in the type declaration is not enforced; add at the end):

```go
type BootOpts struct {
	// ... existing fields ...

	// IDProvider injects a deterministic image-ID source. nil → production default.
	IDProvider image.IDProvider

	// RandReader injects a deterministic random source for OAuth state. nil →
	// crypto/rand.Reader.
	RandReader auth.RandReader
}
```

- [ ] **Step 2: Use the injected providers in `BootApp`.**

Replace the `imgSvc := image.NewService(...)` line with:

```go
ids := opts.IDProvider
if ids == nil {
	ids = image.NewUUIDProvider()
}
imgSvc := image.NewServiceWithIDs(store, images, arts, ids)

if opts.RandReader != nil {
	t.Cleanup(auth.SetStateRandForTest(opts.RandReader))
}
```

(The auth seam is package-global; tests must restore in cleanup. The `t.Cleanup(...)` registration in BootApp ensures parallel-safe behavior is not promised — match the existing `dbtest.StartPostgres` sync.Once invariant.)

- [ ] **Step 3: Build.**

```bash
go build ./...
```

Expected: green.

- [ ] **Step 4: Commit.**

```bash
git add api/internal/dbtest/bootapp.go
git commit -m "[0.7-contract-matrix] feat: wire ID and Rand injection into BootApp"
```

### Task 7.4a — Add the deterministic fake auth provider

**Files:**
- Create: `api/internal/httpapi/contract/fake_provider.go`

The current `auth.Provider` interface has three methods (`auth/provider.go:13-17`): `Name()`, `AuthURL(state) string`, `Exchange(ctx, code) (*Profile, error)`. The contract suite needs a deterministic fake so:

- `/auth/google/start` produces a real 302 redirect (not a 404 from `unknown_provider`).
- `/auth/google/callback` produces a deterministic Profile so the resulting `Set-Cookie auth=…` is stable post-normalization.

- [ ] **Step 1: Write the fake.**

Write `api/internal/httpapi/contract/fake_provider.go`:

```go
// api/internal/httpapi/contract/fake_provider.go
package contract

import (
	"context"

	"local/art-web/api/internal/auth"
)

// fakeGoogleProvider is a deterministic auth.Provider for the contract suite.
// The behavior is chosen to exercise every auth response branch:
//
//   /auth/google/start   → 302 to a fixed AuthURL (stable post-normalization)
//   /auth/google/callback?state=…&code=valid     → 302 to frontend home
//   /auth/google/callback?state=…&code=fail      → 502 exchange_failed
//   /auth/google/callback?state=mismatch          → 400 bad_state
type fakeGoogleProvider struct{}

func (fakeGoogleProvider) Name() string { return "google" }

func (fakeGoogleProvider) AuthURL(state string) string {
	// State is dynamic by definition, but the contract harness already injects a
	// deterministic RandReader, so `state` is stable across runs.
	return "https://example.com/oauth2/auth?state=" + state
}

func (fakeGoogleProvider) Exchange(_ context.Context, code string) (*auth.Profile, error) {
	if code == "fail" {
		return nil, errFakeExchange
	}
	return &auth.Profile{
		Subject:     "fake-subject-1",
		Email:       "fake@example.com",
		DisplayName: "Fake User",
		AvatarURL:   "",
	}, nil
}

// errFakeExchange is the sentinel returned for code=fail; the handler maps any
// non-nil exchange error to 502 exchange_failed.
var errFakeExchange = &exchangeFailure{}

type exchangeFailure struct{}

func (*exchangeFailure) Error() string { return "fake exchange failure" }

// FakeProviders returns the providers map the contract suite hands to BootApp.
// Keep it small: only "google" is wired today.
func FakeProviders() map[string]auth.Provider {
	return map[string]auth.Provider{"google": fakeGoogleProvider{}}
}
```

- [ ] **Step 2: Build to confirm it compiles.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go build ./internal/httpapi/contract/...
```

Expected: green.

- [ ] **Step 3: Commit.**

```bash
git add api/internal/httpapi/contract/fake_provider.go
git commit -m "[0.7-contract-matrix] feat: deterministic fake auth.Provider for contract"
```

### Task 7.4 — Write the full cartesian contract matrix

**Files:**
- Delete: `api/internal/httpapi/contract/placeholder_test.go` (replaced by `matrix_test.go`)
- Create: `api/internal/httpapi/contract/matrix_test.go`

- [ ] **Step 1: Delete the placeholder.**

```bash
rm /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/httpapi/contract/placeholder_test.go
```

- [ ] **Step 2: Write the full matrix file.**

The matrix enumerates one entry per cell of (auth × resource × request shape) per the table in spec §5.2. The test loops over the matrix and calls `dbtest.AssertGolden`. Because no goldens exist yet, every assertion fails — that is expected. PR 0.8 turns those failures into committed snapshots.

**Critical wiring:**
- The HTTP client uses `CheckRedirect: http.ErrUseLastResponse` so 302 redirects are captured as 302, not followed silently.
- `BootApp` receives `Providers: contract.FakeProviders()` so `/auth/google/start` produces a real 302 instead of a 404.

Write `api/internal/httpapi/contract/matrix_test.go`:

```go
package contract_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/httpapi/contract"
	"local/art-web/api/internal/image"
)

// fixedNow is the deterministic clock for every contract test.
var fixedNow = time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

// fixedRand returns a deterministic 16-byte source for OAuth state.
type fixedRand struct{}

func (fixedRand) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i + 1)
	}
	return len(p), nil
}

// seedResponse mirrors the /dev/seed response shape (httpapi/devseed.go:57-64).
type seedResponse struct {
	AliceCookie string `json:"aliceCookie"`
	BobCookie   string `json:"bobCookie"`
	AliceSlug   string `json:"aliceSlug"`
	PID         string `json:"pId"` // public artwork
	QID         string `json:"qId"` // private artwork
}

type contractCase struct {
	name     string
	method   string
	path     string
	viewer   string // "anon", "owner", "other"
	body     string // body bytes; empty allowed
	ctype    string // Content-Type override; if empty and body != "" defaults to application/json
	bodyMP   func(t *testing.T) (io.Reader, string) // optional: builds a multipart body & returns ctype
	cookies  []*http.Cookie                          // optional: extra cookies (e.g. oauth_state for callback cells)
}

// fixedOAuthState is what randState() returns when stateRand is fixedRand{}.
// fixedRand fills 16 bytes with 0x01..0x10; hex-encoded that's
// "0102030405060708090a0b0c0d0e0f10". Cells targeting /auth/google/callback
// pre-set this cookie so the handler's state-match check passes.
const fixedOAuthState = "0102030405060708090a0b0c0d0e0f10"

// noRedirectClient prevents auto-following 302s so we capture the redirect
// as the actual response — auth/start and auth/callback both 302.
var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func bootContract(t *testing.T) (string, seedResponse, func(c contractCase) *http.Response) {
	srv := dbtest.BootApp(t, dbtest.BootOpts{
		FixedNow:   fixedNow,
		IDProvider: &image.CounterIDProvider{},
		RandReader: fixedRand{},
		Providers:  contract.FakeProviders(),
	})

	// Seed once via /dev/seed?suffix=fixed1234 — deterministic suffix so the
	// resulting AliceSlug/BobSlug are stable across runs (round-2 Finding 4).
	r, err := noRedirectClient.Post(srv.URL+"/dev/seed?suffix=fixed1234", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /dev/seed: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("seed status=%d want 200", r.StatusCode)
	}
	var seed seedResponse
	if err := json.NewDecoder(r.Body).Decode(&seed); err != nil {
		t.Fatalf("decode seed: %v", err)
	}

	send := func(c contractCase) *http.Response {
		var bodyReader io.Reader
		var ctype string
		switch {
		case c.bodyMP != nil:
			bodyReader, ctype = c.bodyMP(t)
		case c.body != "":
			bodyReader = strings.NewReader(c.body)
			ctype = "application/json"
		default:
			bodyReader = nil
		}
		if c.ctype != "" {
			ctype = c.ctype
		}
		req, _ := http.NewRequest(c.method, srv.URL+c.path, bodyReader)
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		switch c.viewer {
		case "owner":
			req.AddCookie(&http.Cookie{Name: "auth", Value: seed.AliceCookie})
		case "other":
			req.AddCookie(&http.Cookie{Name: "auth", Value: seed.BobCookie})
		case "anon":
			// no cookie
		default:
			t.Fatalf("unknown viewer %q", c.viewer)
		}
		for _, ck := range c.cookies {
			req.AddCookie(ck)
		}
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("do %s %s: %v", c.method, c.path, err)
		}
		return resp
	}

	return srv.URL, seed, send
}

// validImageUpload returns a multipart body builder that uploads a tiny PNG.
// The PNG bytes are the same 1×1 transparent PNG used in
// httpapi/testutil_test.go:28-37 — base64-decoded once at init time.
func validImageUpload(clientID string, position int) func(*testing.T) (io.Reader, string) {
	return func(t *testing.T) (io.Reader, string) {
		buf := &bytes.Buffer{}
		mw := multipart.NewWriter(buf)
		manifest := `[{"client_image_id":"` + clientID + `","position":` +
			strconv.Itoa(position) + `,"content_type":"image/png"}]`
		_ = mw.WriteField("manifest", manifest)
		w, _ := mw.CreateFormFile("files", clientID+".png")
		_, _ = w.Write(seedPNGBytes)
		_ = mw.Close()
		return buf, mw.FormDataContentType()
	}
}

// seedPNGBytes is a 1×1 transparent PNG (67 bytes), copied from testutil_test.go.
var seedPNGBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c, 0x02, 0x00, 0x00, 0x00,
	0x0b, 0x49, 0x44, 0x41, 0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
	0x03, 0x03, 0x02, 0x00, 0xef, 0xbf, 0xa7, 0xdb, 0x00, 0x00, 0x00, 0x00,
	0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// TestContractMatrix enumerates every (auth × resource × shape) cell from spec
// §5.2. Each cell asserts against a checked-in golden. Until PR 0.8 these fail.
func TestContractMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("contract suite is slow; run without -short")
	}
	_, seed, send := bootContract(t)

	missingID := "00000000-0000-0000-0000-000000000000"
	cases := []contractCase{
		// === /healthz ===
		{name: "healthz_anon", method: "GET", path: "/healthz", viewer: "anon"},

		// === /me ===
		{name: "me_anon_401", method: "GET", path: "/me", viewer: "anon"},
		{name: "me_owner_200", method: "GET", path: "/me", viewer: "owner"},
		{name: "me_other_200", method: "GET", path: "/me", viewer: "other"},

		// === GET /artworks (feed) ===
		{name: "feed_anon", method: "GET", path: "/artworks", viewer: "anon"},
		{name: "feed_owner", method: "GET", path: "/artworks", viewer: "owner"},
		{name: "feed_anon_limit_1", method: "GET", path: "/artworks?limit=1", viewer: "anon"},
		{name: "feed_anon_limit_oob", method: "GET", path: "/artworks?limit=9999", viewer: "anon"},
		{name: "feed_anon_bad_cursor", method: "GET", path: "/artworks?cursor=not-base64!!", viewer: "anon"},

		// === POST /artworks ===
		{name: "create_anon_401", method: "POST", path: "/artworks", viewer: "anon", body: `{"title":"x"}`},
		{name: "create_owner_minimal", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x"}`},
		{name: "create_owner_full", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x","description":"d","visibility":"public","tags":["a","b"]}`},
		{name: "create_owner_default_visibility", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x","visibility":""}`},
		{name: "create_owner_bad_json", method: "POST", path: "/artworks", viewer: "owner", body: `not json`},
		{name: "create_owner_bad_visibility", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x","visibility":"draft"}`},

		// === GET /artworks/{id} — full 7-cell privacy matrix ===
		{name: "get_artwork_owner_public", method: "GET", path: "/artworks/" + seed.PID, viewer: "owner"},
		{name: "get_artwork_other_public", method: "GET", path: "/artworks/" + seed.PID, viewer: "other"},
		{name: "get_artwork_anon_public", method: "GET", path: "/artworks/" + seed.PID, viewer: "anon"},
		{name: "get_artwork_owner_private", method: "GET", path: "/artworks/" + seed.QID, viewer: "owner"},
		{name: "get_artwork_other_private_404", method: "GET", path: "/artworks/" + seed.QID, viewer: "other"},
		{name: "get_artwork_anon_private_404", method: "GET", path: "/artworks/" + seed.QID, viewer: "anon"},
		{name: "get_artwork_anon_missing", method: "GET", path: "/artworks/" + missingID, viewer: "anon"},

		// === PATCH /artworks/{id} ===
		{name: "patch_anon_401", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "anon", body: `{"title":"x"}`},
		{name: "patch_owner_title_204", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"title":"renamed"}`},
		{name: "patch_owner_same_visibility_204", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"visibility":"public"}`},
		{name: "patch_owner_flip_to_private_204", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"visibility":"private"}`},
		{name: "patch_owner_full_204", method: "PATCH", path: "/artworks/" + seed.QID, viewer: "owner", body: `{"title":"t","description":"d","cover_position":0,"tags":["a"]}`},
		{name: "patch_owner_bad_json", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `not json`},
		{name: "patch_owner_bad_visibility", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"visibility":"draft"}`},
		{name: "patch_owner_bad_cover_position", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"cover_position":-1}`},
		{name: "patch_other_public_404", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "other", body: `{"title":"x"}`},
		{name: "patch_other_private_404", method: "PATCH", path: "/artworks/" + seed.QID, viewer: "other", body: `{"title":"x"}`},

		// === DELETE /artworks/{id} ===
		{name: "delete_anon_401", method: "DELETE", path: "/artworks/" + seed.PID, viewer: "anon"},
		{name: "delete_other_public_404", method: "DELETE", path: "/artworks/" + seed.PID, viewer: "other"},
		{name: "delete_other_private_404", method: "DELETE", path: "/artworks/" + seed.QID, viewer: "other"},
		{name: "delete_owner_missing_404", method: "DELETE", path: "/artworks/" + missingID, viewer: "owner"},

		// === POST /artworks/{id}/images ===
		{name: "upload_anon_401", method: "POST", path: "/artworks/" + seed.PID + "/images", viewer: "anon",
			ctype: "multipart/form-data; boundary=fake"},
		{name: "upload_other_404", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "other",
			ctype: "multipart/form-data; boundary=fake"},
		{name: "upload_owner_415_non_multipart", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			body: `{}`},
		{name: "upload_owner_400_no_manifest", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			bodyMP: func(t *testing.T) (io.Reader, string) {
				buf := &bytes.Buffer{}
				mw := multipart.NewWriter(buf)
				_ = mw.Close()
				return buf, mw.FormDataContentType()
			}},
		{name: "upload_owner_400_file_count_mismatch", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			bodyMP: func(t *testing.T) (io.Reader, string) {
				buf := &bytes.Buffer{}
				mw := multipart.NewWriter(buf)
				_ = mw.WriteField("manifest", `[{"client_image_id":"K","position":0,"content_type":"image/png"}]`)
				_ = mw.Close()
				return buf, mw.FormDataContentType()
			}},
		{name: "upload_owner_201", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			bodyMP: validImageUpload("K-new", 1)},

		// === /users/{slug} ===
		{name: "user_profile_owner_self", method: "GET", path: "/users/" + seed.AliceSlug, viewer: "owner"},
		{name: "user_profile_anon", method: "GET", path: "/users/" + seed.AliceSlug, viewer: "anon"},
		{name: "user_profile_missing_slug", method: "GET", path: "/users/no-such-slug", viewer: "anon"},
		{name: "user_profile_bad_cursor", method: "GET", path: "/users/" + seed.AliceSlug + "?cursor=not-base64!!", viewer: "anon"},

		// === /tags/{name} ===
		{name: "tag_present", method: "GET", path: "/tags/t", viewer: "anon"},
		{name: "tag_missing", method: "GET", path: "/tags/no-such-tag", viewer: "anon"},

		// === /auth/{provider}/start — fake provider produces 302 ===
		{name: "auth_google_start_anon_302", method: "GET", path: "/auth/google/start", viewer: "anon"},
		{name: "auth_unknown_provider_404", method: "GET", path: "/auth/notreal/start", viewer: "anon"},

		// === /auth/{provider}/callback ===
		// fixedRand makes randState() deterministic; the state cookie value is
		// known at compile time (fixedOAuthState). Cells that exercise the
		// state-match branch pre-set the cookie.
		{name: "auth_callback_unknown_provider_404", method: "GET",
			path: "/auth/notreal/callback?state=" + fixedOAuthState + "&code=y", viewer: "anon"},
		{name: "auth_callback_bad_state_400", method: "GET",
			path:    "/auth/google/callback?state=mismatch&code=y",
			viewer:  "anon",
			cookies: []*http.Cookie{{Name: "oauth_state", Value: fixedOAuthState}},
		},
		// Happy path: state cookie matches query, fake provider returns success → 302 to Frontend.
		{name: "auth_callback_success_302", method: "GET",
			path:    "/auth/google/callback?state=" + fixedOAuthState + "&code=valid",
			viewer:  "anon",
			cookies: []*http.Cookie{{Name: "oauth_state", Value: fixedOAuthState}},
		},
		// Exchange failure: state matches, fake provider's Exchange returns error → 502 exchange_failed.
		{name: "auth_callback_exchange_failed_502", method: "GET",
			path:    "/auth/google/callback?state=" + fixedOAuthState + "&code=fail",
			viewer:  "anon",
			cookies: []*http.Cookie{{Name: "oauth_state", Value: fixedOAuthState}},
		},

		// === /auth/logout ===
		{name: "logout_anon_204", method: "POST", path: "/auth/logout", viewer: "anon"},
		{name: "logout_owner_204", method: "POST", path: "/auth/logout", viewer: "owner"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := send(c)
			defer resp.Body.Close()
			dbtest.AssertGolden(t, c.name, resp)
		})
	}
}
```

> The matrix covers every (auth × resource × shape) cell from spec §5.2 except: (a) `/dev/seed` (excluded by §5.2.1), (b) the auth-callback happy path (requires cookie pre-seed beyond what fake_provider provides cleanly — captured by inspecting whether the body bytes change in PR 0.8). If PR 0.8's capture surfaces additional cells that need explicit handling, add them and re-run with `GOLDEN_UPDATE=1`.

- [ ] **Step 3: Run.**

Run: `go test ./internal/httpapi/contract/... -v`
Expected: every subtest fails with "rerun with GOLDEN_UPDATE=1 to create" — that is the intended state until PR 0.8.

- [ ] **Step 4: Commit (and stage the deletion).**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web
git add api/internal/httpapi/contract/matrix_test.go
git rm api/internal/httpapi/contract/placeholder_test.go
git commit -m "[0.7-contract-matrix] feat: enumerate cartesian HTTP contract cells (no goldens yet)"
```

### Task 7.5 — Write the forbidden-status meta-test

**Files:**
- Create: `api/internal/httpapi/contract/forbidden_status_test.go`

- [ ] **Step 1: Write the meta-test.**

Per spec §5.2.0a, this test scans every captured `golden/*.json` file and fails if any forbidden status (403, translated 409, etc.) appears.

Write `api/internal/httpapi/contract/forbidden_status_test.go`:

```go
package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Forbidden statuses per spec §5.2.0a: the current API never emits 403,
// non-owner access on private resources collapses to 404. 410/451/511 are
// not emitted by any current handler; introducing them would be a contract
// change. 409 from translated domain errors (e.g., ErrAlreadyPublished) is
// forbidden; the only legitimate 409 is image fingerprint_mismatch, asserted
// by name below.
var forbiddenStatuses = map[int]string{
	403: "non-owner access must collapse to 404 (spec §7.2.1)",
	410: "no current handler emits 410",
	451: "no current handler emits 451",
	511: "no current handler emits 511",
}

type goldenEnvelope struct {
	Body    string              `json:"body"`
	Headers map[string][]string `json:"headers"`
	Status  int                 `json:"status"`
}

func TestForbiddenStatuses_AbsentFromAllGoldens(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("golden", "*.json"))
	if err != nil {
		t.Fatalf("glob goldens: %v", err)
	}
	if len(matches) == 0 {
		t.Skip("no goldens captured yet (PR 0.8 is what populates this)")
	}

	for _, m := range matches {
		bs, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		var env goldenEnvelope
		if err := json.Unmarshal(bs, &env); err != nil {
			t.Fatalf("parse %s: %v", m, err)
		}
		if reason, bad := forbiddenStatuses[env.Status]; bad {
			t.Errorf("golden %s emits forbidden status %d: %s", m, env.Status, reason)
		}
		// 409 with a body NOT containing "fingerprint_mismatch" is a translated
		// domain error and must be absent.
		if env.Status == 409 && !contains409Allowed(env.Body) {
			t.Errorf("golden %s emits 409 without fingerprint_mismatch; "+
				"likely a translated domain error (spec §7.2.1)", m)
		}
	}
}

func contains409Allowed(body string) bool {
	// The only legitimate 409 in the current API is image fingerprint_mismatch.
	// The body must contain exactly that error code.
	const expected = `"error":"fingerprint_mismatch"`
	for i := 0; i+len(expected) <= len(body); i++ {
		if body[i:i+len(expected)] == expected {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run — should pass-with-skip until goldens exist.**

Run: `go test ./internal/httpapi/contract/... -run TestForbiddenStatuses_AbsentFromAllGoldens -v`
Expected: `--- SKIP` (no goldens captured yet).

- [ ] **Step 3: Commit.**

```bash
git add api/internal/httpapi/contract/forbidden_status_test.go
git commit -m "[0.7-contract-matrix] feat: meta-test forbids 403 and translated 409 in goldens"
```

### Task 7.6 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.7-contract-matrix
gh pr create --base master --title "[0.7-contract-matrix] HTTP contract matrix + deterministic injection" --body "$(cat <<'EOF'
## Summary
- (Confirms `image.IDProvider` + `CounterIDProvider` already shipped in PR 0.4.)
- Add `auth.RandReader` interface; switch `randState` to use it. With `fixedRand{}` injecting bytes 0x01..0x10, the OAuth state value is the constant \`0102030405060708090a0b0c0d0e0f10\`.
- Add `?suffix=` query parameter to `/dev/seed` so the contract suite uses a fixed slug suffix (\`alice-fixed1234\`, \`bob-fixed1234\`).
- Wire `IDProvider` + `RandReader` + `Providers` + fixed clock into `dbtest.BootApp`.
- Add `contract.FakeProviders()` with deterministic 302/502/400 branches so `/auth/google/start` produces a real 302 and `/auth/google/callback` exercises every status.
- Replace placeholder_test.go with the full cartesian contract matrix covering every (auth × resource × shape) cell from spec §5.2 — including auth-callback success (302) and exchange_failed (502) cells with pre-set \`oauth_state\` cookie.
- Use `noRedirectClient` (`CheckRedirect: http.ErrUseLastResponse`) so 302s are captured as 302, not silently followed.
- Write the forbidden-status meta-test (skips until goldens exist).

## Test plan
- [ ] All existing tests still pass (the RandReader switch is a no-op refactor; the devseed `?suffix=` change defaults to current behavior)
- [ ] `go test ./internal/httpapi/contract/...` fails with golden-not-found errors for every cell (intended; PR 0.8 captures)
- [ ] `go vet ./...` clean
- [ ] `auth_google_start_anon_302` cell hits the fake provider's 302 (not the unknown_provider 404)
- [ ] `auth_callback_success_302` and `auth_callback_exchange_failed_502` cells exercise both fake-provider branches

Refs spec §5.2 (matrix), §5.2.0a (forbidden statuses), §5.3.2 (dynamic-byte injection).
EOF
)"
```

---

## PR 0.8 — Lock the snapshots

**Branch:** `phase0/0.8-snapshot-lock`

**Goal:** Capture every contract cell as a checked-in golden file, activate the `contract_suite` CI job as required, add CODEOWNERS protection on `golden/`, and confirm `/dev/seed` is excluded.

**Acceptance criteria:**
- `golden/` is non-empty and committed.
- `make test-contract` is green at HEAD without `GOLDEN_UPDATE=1`.
- `contract_suite` CI job is required on all PRs.
- CODEOWNERS protects `api/internal/httpapi/contract/golden/`.
- The forbidden-status meta-test passes (no longer skips).
- `/dev/seed` has no golden file.

### Task 8.1 — Capture the snapshots

**Files:**
- Create: `api/internal/httpapi/contract/golden/*.json` (one per case in `matrix_test.go`)

- [ ] **Step 1: Run with GOLDEN_UPDATE=1.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
GOLDEN_UPDATE=1 go test ./internal/httpapi/contract/... -v
```

Expected: every subtest passes (write mode never compares). The `golden/` directory is created with one `<case-name>.json` per case.

- [ ] **Step 2: Inspect a few snapshots to confirm normalization is correct.**

```bash
cat /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/httpapi/contract/golden/feed_anon_empty.json | head -20
cat /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/httpapi/contract/golden/auth_unknown_provider.json | head -20
```

UUIDs should appear as `<UUID>`; timestamps as `<TIMESTAMP>`. JWT cookie values (Set-Cookie auth=...) should appear as `<JWT>`.

- [ ] **Step 3: Confirm `/dev/seed` is NOT snapshotted.**

```bash
ls /Users/todd.lam/WORK/_TestScripts/art-web/api/internal/httpapi/contract/golden/ | grep -i seed && echo "FAIL: dev/seed should be absent" || echo "OK: no dev/seed snapshot"
```

If a `seed_*` file appears, remove the corresponding case from `matrix_test.go` and re-capture.

- [ ] **Step 4: Re-run without `GOLDEN_UPDATE` to confirm replay passes byte-strict.**

```bash
go test ./internal/httpapi/contract/... -v
```

Expected: every subtest passes. The forbidden-status meta-test now does NOT skip.

- [ ] **Step 5: Commit the snapshots.**

```bash
git add api/internal/httpapi/contract/golden/
git commit -m "[0.8-snapshot-lock] feat: capture HTTP contract goldens"
```

### Task 8.2 — Activate the CI job

**Files:**
- Modify: `.github/workflows/contract.yml` (or its equivalent)

- [ ] **Step 1: Remove the `if: false` gate.**

Open `.github/workflows/contract.yml` and delete this line:

```
    if: false   # disabled until PR 0.8 captures goldens
```

The job now runs on every PR.

- [ ] **Step 2: Add it to required checks.**

Mark `contract_suite` as required via the GitHub UI: **Settings → Branches → Branch protection rule → master → Require status checks → contract_suite**. The PR description should include a screenshot or note confirming the team has done this.

- [ ] **Step 3: Commit.**

```bash
git add .github/workflows/contract.yml
git commit -m "[0.8-snapshot-lock] chore: activate contract_suite CI job"
```

### Task 8.3 — Add CODEOWNERS protection on `golden/`

**Files:**
- Modify: `CODEOWNERS` (root or `.github/CODEOWNERS` — verify with `ls /Users/todd.lam/WORK/_TestScripts/art-web/.github/`)

- [ ] **Step 1: Add the protection line.**

Append to the CODEOWNERS file:

```
# Contract snapshots — changes require backend lead review.
api/internal/httpapi/contract/golden/   @backend-leads
```

> Replace `@backend-leads` with the GitHub team or username the team agreed on (spec §13 open question 2). If the team or user does not exist yet, leave a TODO comment and ship the line as `@todd-l-am` (the current backend committer per `git log --format='%an' | sort -u`) so the protection is real.

- [ ] **Step 2: Commit.**

```bash
git add CODEOWNERS
git commit -m "[0.8-snapshot-lock] chore: protect contract goldens via CODEOWNERS"
```

### Task 8.4 — Build the validator-translator table reference

**Files:**
- Create: `api/internal/httpapi/contract/error_codes_observed.md`

- [ ] **Step 1: Extract the unique `error` codes from the captured snapshots.**

The body is stored as a JSON-escaped string inside each envelope, so a raw `grep '"error":"…"'` won't match (escaped quotes break the pattern — round-2 Finding 7). Use `jq` to decode the envelope, parse `body` as JSON, and extract the `error` field:

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
for f in internal/httpapi/contract/golden/*.json; do
  jq -r '
    {status: .status,
     err:    (.body | try fromjson | .error // empty)}
    | select(.err != "")
    | "\(.status)\t\(.err)"
  ' "$f"
done | sort -u
```

Output is `<status>\t<code>` lines, one per unique pair. `try fromjson` skips non-JSON bodies (e.g., 302 redirects with empty body). Pipe through `column -t` for easier reading.

If `jq` isn't installed, the equivalent Go one-liner is:

```bash
go run - <<'EOF'
package main
import (
  "encoding/json"; "fmt"; "os"; "path/filepath"; "sort"
)
type env struct{ Status int; Body string }
func main() {
  paths, _ := filepath.Glob("internal/httpapi/contract/golden/*.json")
  seen := map[string]bool{}
  for _, p := range paths {
    bs, _ := os.ReadFile(p); var e env; json.Unmarshal(bs, &e)
    var b struct{ Error string `json:"error"` }
    if json.Unmarshal([]byte(e.Body), &b) == nil && b.Error != "" {
      seen[fmt.Sprintf("%d\t%s", e.Status, b.Error)] = true
    }
  }
  out := make([]string, 0, len(seen)); for k := range seen { out = append(out, k) }
  sort.Strings(out); for _, k := range out { fmt.Println(k) }
}
EOF
```

- [ ] **Step 2: Write the reference table that PR 0.8 hands off to Phase 1's plan author.**

Write `api/internal/httpapi/contract/error_codes_observed.md`:

```markdown
# Observed error codes (Phase 0 snapshot)

This table is the canonical input to Phase 1's `WriteError` switch (spec §7.4)
and validator translator (§7.5). Every code listed here is byte-locked into a
golden file. Phase 1 must emit exactly these codes for these statuses.

| Status | Error code               | Shape | Source endpoint(s) |
|--------|--------------------------|-------|--------------------|
| <fill in by reading goldens; one row per unique (status, error code) pair> |  |  |  |

> Generation step: `grep -hoE '"error":"[^"]+"' golden/*.json | sort -u`
```

Fill the rows by inspecting the goldens. Examples for the cells captured in PR 0.7:
- `(401, "unauthorized", A, /me, POST/PATCH/DELETE artworks)`
- `(404, "not_found", A, GET/PATCH/DELETE artwork by id, GET user, GET tag)`
- `(404, "unknown_provider", A, /auth/{provider}/{start,callback})`
- `(400, "bad_cursor", B, GET /artworks)`
- ...

- [ ] **Step 3: Commit.**

```bash
git add api/internal/httpapi/contract/error_codes_observed.md
git commit -m "[0.8-snapshot-lock] docs: snapshot-derived error-code table for Phase 1"
```

### Task 8.5 — Open the PR

- [ ] **Step 1: Push and open.**

```bash
git push -u origin phase0/0.8-snapshot-lock
gh pr create --base master --title "[0.8-snapshot-lock] Lock HTTP contract snapshots" --body "$(cat <<'EOF'
## Summary
- Capture every contract cell as a committed golden file.
- Activate `contract_suite` CI as a required check.
- Add CODEOWNERS protection on `api/internal/httpapi/contract/golden/`.
- Confirm `/dev/seed` is excluded.
- Add reference table of every observed error code as input to Phase 1.

## Test plan
- [ ] `make test-contract` green
- [ ] `golden/` has the expected count of files, none with `seed_*` prefix
- [ ] forbidden-status meta-test passes (no longer skips)
- [ ] CODEOWNERS team confirmed before merge

Refs spec §5.1, §5.2.0a, §5.2.1, §8.1 PR 0.8.

> 🛑 After this PR merges, Phase 0 is complete. Phase 1 gets its own plan,
> written from `error_codes_observed.md` and the captured goldens.
EOF
)"
```

---

## Phase 0 hard gates (block Phase 1 from starting)

Per spec §8.3, before the Phase 1 branch is created, all of the following must be true:

- [ ] PR 0.8 merged to master.
- [ ] `make test-cover-slices` shows all four business slices ≥ 80 % on the old package paths.
- [ ] `contract_suite` CI job exists, is required, and is currently green on master.
- [ ] `api/internal/httpapi/contract/golden/` exists, is non-empty, has CODEOWNERS protection.
- [ ] No frontend changes are planned during Phase 1's branch lifetime, OR a coordination owner is identified.
- [ ] Branch protection on master forbids force-push.

When all six gates are checked, the team is ready to write the Phase 1 plan.

---

## What happens after Phase 0

**Phase 1 gets its own plan**, written after PR 0.8 lands. The reason is mechanical, not procedural: spec §7.4 and §7.5 say the validator-translator table and the full error-code mapping are *built from* PR 0.8's captured snapshots. Writing those before the snapshots exist would mean placeholders.

The Phase 1 plan will be saved to `docs/superpowers/plans/2026-XX-XX-go-backend-refactor-phase1.md` and will reference:
- `api/internal/httpapi/contract/error_codes_observed.md` for the canonical error-code list.
- The committed goldens as the byte-strict target.
- Spec §6 (Wire DI graph), §7 (domain & error-handling conventions), §9 (commit-ordering constraints), §10 (merge gates).

The Phase 1 plan author should re-invoke `superpowers:writing-plans` with the spec + the contract artifacts in hand.
