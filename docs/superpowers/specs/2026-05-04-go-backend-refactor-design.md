# Go backend refactor: hex-arch + Wire + GORM + zerolog

**Date:** 2026-05-04
**Status:** Design — awaiting user review before plan generation
**Scope:** `api/` Go module only (worker/ is TypeScript; out of scope)
**Reference:** [github.com/cuongtranba/go-scaffolding](https://github.com/cuongtranba/go-scaffolding)

---

## 1. Goals & non-goals

### Goals

- Adopt a feature-sliced hexagonal layout in `api/internal/` modeled on the reference repo.
- Replace pgx with GORM, slog with zerolog, manual DI with Google Wire.
- Reach **≥ 80% statement coverage** on the four business slices (`auth`, `user`, `artwork`, `image`) before any structural change lands.
- Lock observable behavior with a byte-strict HTTP contract suite that runs identically before and after the refactor — the refactor must produce **zero observable side effects** at the HTTP boundary.

### Non-goals

- No new endpoints. No API contract changes (request shapes, response shapes, or error wording).
- No schema changes (migrations stay frozen; GORM tags must match existing columns exactly).
- No frontend coordination beyond announcing the freeze window.
- No performance optimization unless required to make a snapshot pass.
- No new feature slices (`comments`, `likes`, etc.) — those are separate work.

---

## 2. Decision snapshot

These five decisions, locked during brainstorming, drive the rest of the spec:

| # | Decision | Choice |
|---|---|---|
| 1 | Adoption strictness | **C-strict** — full reference adoption (layout + Wire + GORM + zerolog) |
| 2 | Coverage interpretation | **D** — 80% statement coverage on business slices *plus* HTTP behavioral contract suite |
| 3 | Feature slicing | **B (revised)** — 4 slices (`auth`, `user`, `artwork`, `image`) + `pkg/signing` + `infrastructure/` + `cmd/seeder`. *Storage is infrastructure, not part of the image slice.* |
| 4 | Execution sequence | **A** — big-bang refactor in one branch (after Phase 0 lands) |
| 5 | Contract suite shape | **A** — HTTP-only black-box; testcontainers Postgres + minio; byte-strict snapshot comparison |

---

## 3. Current state

### Layout (today)

```
api/
├── cmd/api/                       (main.go, config.go, main_test.go)
├── migrations/                    (SQL via golang-migrate)
└── internal/
    ├── artwork/                   (repo.go, tags.go, visibility.go — domain + persistence mixed)
    ├── auth/                      (provider.go, handlers.go, jwt.go, sign.go, middleware.go,
    │                               google.go, imgurl.go — identity + URL signing mixed)
    ├── user/                      (repo.go)
    ├── image/                     (handler.go, service.go, repo.go, decode.go, manifest.go
    │                               — already loosely 3-layered)
    ├── httpapi/                   (router.go, artworks.go, users.go, tags.go,
    │                               middleware.go, render.go, devseed.go [605 lines])
    ├── storage/                   (storage.go interface + localfs.go + r2.go adapters)
    ├── db/                        (pool.go, migrate.go)
    └── dbtest/                    (postgres.go — testcontainers harness)
```

### Coverage baseline (measured 2026-05-04)

| Package | Coverage | Gap to 80% |
|---|---|---|
| `auth` | 81.8% | already met |
| `artwork` | 78.3% | -1.7% |
| `db` | 75.0% | -5.0% |
| `image` | 69.7% | -10.3% |
| `httpapi` | 69.2% | -10.8% |
| `storage` | 69.1% | -10.9% |
| `user` | 63.8% | -16.2% |
| `cmd/api` | 1.8% | excluded (wiring code) |

### Stack baseline

- **DB driver:** `jackc/pgx/v5`
- **Logger:** `log/slog` (stdlib)
- **DI:** manual wiring in `cmd/api/main.go`
- **HTTP router:** `go-chi/chi/v5`
- **Migrations:** `golang-migrate/migrate/v4` (SQL files)
- **OAuth:** `golang.org/x/oauth2` (Google only)
- **Object storage:** AWS SDK v2 → R2 / MinIO / local filesystem
- **JWT:** `golang-jwt/jwt/v5`

### Current error response shape (locked from current handlers)

The current API emits errors as JSON in two distinct shapes. Both are part of the contract:

```json
// Shape A — most paths
{ "error": "<snake_case_code>" }

// Shape B — paths that surface a detail message
{ "error": "<snake_case_code>", "message": "<detail>" }
```

Content-Type: `application/json; charset=utf-8`.

**Shape-A paths (verified — stable codes):** `unauthorized`, `not_found`, `bad_json`, `bad_visibility`, `bad_cover_position`, `create_failed`, `patch_failed`, `flip_failed`, `tag_failed`, `delete_failed`, `list_failed`, `user_lookup_failed`, `user_failed`, `unknown_provider` (auth), `bad_state` (auth), `exchange_failed` (auth), `upsert_failed` (auth), `sign_failed` (auth), `unsupported_media_type` (image), `bad_multipart` (image), `manifest_required` (image), `file_count_mismatch` (image), `open_file` (image), `position_taken` (image), `too_large` (image), `decode_failed` (image), `upload_failed` (image).

**Shape-A quirky variant (verified — `error` value is a free-form string, not a stable code):**
- ParseManifest errors — `image/handler.go:51` — emits `{"error": err.Error()}` where `err.Error()` is the parse-error string itself (e.g., `"manifest: line 3: invalid client_image_id"`). No `message` key. This is a current API quirk that's frozen into the contract until a future cleanup PR.

**Shape-B paths (verified — stable code + detail message):**
- `bad_cursor` — `httpapi/artworks.go:87` — `message` is the parse-error string
- `fingerprint_mismatch` — `image/handler.go:72-75` — `message: "client_image_id reused with different bytes"`
- `content_type_mismatch` — `image/handler.go:84-87` — `message: "body does not match declared content_type"`

The Phase 1 implementation reproduces all of these exactly.

> Note: codes like `*_failed` leak internal operation names. They are frozen into the contract; cleanup is a post-Phase-1 follow-up.

---

## 4. Target architecture

### 4.1 Layout

```
api/
├── cmd/
│   ├── api/                       (main.go + wire.go + wire_gen.go + main_test.go)
│   └── seeder/                    (extracted from internal/httpapi/devseed.go)
├── migrations/                    (unchanged — GORM still uses golang-migrate SQL files)
├── pkg/
│   └── signing/                   (HMAC URL signer; shared with worker/ for verification)
└── internal/
    ├── auth/
    │   ├── domain/                (Identity, Session, Provider, errors)
    │   ├── ports/                 (UserLookup, OAuthProvider, JWTIssuer)
    │   ├── service/               (login orchestration, callback flow, JWT issue/verify)
    │   ├── adapters/
    │   │   ├── http/              (handlers, dto, routes, middleware)
    │   │   └── oauth/             (google.go — implements ports.OAuthProvider)
    │   └── providers.go           (wire.NewSet)
    ├── user/
    │   ├── domain/                (User, Profile, errors)
    │   ├── ports/                 (UserRepository)
    │   ├── service/               (profile service)
    │   ├── adapters/
    │   │   ├── http/              (handlers, dto, routes)
    │   │   └── postgres/          (gorm_models, mappers, repo)
    │   └── providers.go
    ├── artwork/
    │   ├── domain/                (Artwork, Tag, Visibility, errors)
    │   ├── ports/                 (ArtworkRepository, TagRepository, RollbackReporter)
    │   ├── service/               (CRUD, tagging, visibility-flip orchestration)
    │   ├── adapters/
    │   │   ├── http/              (handlers, dto, routes)
    │   │   └── postgres/          (gorm_models, mappers, repo, tag_repo)
    │   └── providers.go
    ├── image/
    │   ├── domain/                (Image, Manifest, errors)
    │   ├── ports/                 (ImageRepository, URLSigner)
    │   ├── service/               (upload, decode, manifest, blurhash)
    │   ├── adapters/
    │   │   ├── http/              (handlers, dto, routes)
    │   │   └── postgres/          (gorm_models, mappers, repo)
    │   └── providers.go
    └── infrastructure/
        ├── config/                (env loading; replaces cmd/api/config.go)
        ├── database/              (GORM connection, migrate runner, Transactor)
        ├── logger/                (zerolog setup)
        ├── server/                (chi composition, route-registrar collector,
        │                           WriteError, validator setup)
        ├── storage/               (Storage port + localfs.go + r2.go adapters
        │                           — used by both artwork and image)
        ├── health/                (healthz handler)
        └── testing/               (testcontainers harness, golden-file machinery)
```

### 4.2 File-move table

| Current location | Target location |
|---|---|
| `internal/artwork/repo.go` | `internal/artwork/adapters/postgres/repo.go` (rewritten in GORM) |
| `internal/artwork/tags.go` | `internal/artwork/adapters/postgres/tag_repo.go` + `domain/tag.go` |
| `internal/artwork/visibility.go` | `internal/artwork/service/visibility.go` |
| `internal/auth/handlers.go` | `internal/auth/adapters/http/handlers.go` |
| `internal/auth/middleware.go` | `internal/auth/adapters/http/middleware.go` |
| `internal/auth/jwt.go` | `internal/auth/service/jwt.go` |
| `internal/auth/sign.go` | `pkg/signing/hmac.go` |
| `internal/auth/imgurl.go` | `pkg/signing/imgurl.go` (worker reads same contract) |
| `internal/auth/google.go` | `internal/auth/adapters/oauth/google.go` |
| `internal/auth/provider.go` | `internal/auth/ports/oauth.go` |
| `internal/user/repo.go` | `internal/user/adapters/postgres/repo.go` |
| `internal/image/handler.go` | `internal/image/adapters/http/handler.go` |
| `internal/image/service.go` | `internal/image/service/upload.go` |
| `internal/image/repo.go` | `internal/image/adapters/postgres/repo.go` |
| `internal/image/decode.go` | `internal/image/service/decode.go` |
| `internal/image/manifest.go` | `internal/image/domain/manifest.go` |
| `internal/storage/storage.go` | `internal/infrastructure/storage/ports.go` |
| `internal/storage/localfs.go` | `internal/infrastructure/storage/localfs.go` |
| `internal/storage/r2.go` | `internal/infrastructure/storage/r2.go` |
| `internal/httpapi/router.go` | `internal/infrastructure/server/router.go` |
| `internal/httpapi/render.go` | `internal/infrastructure/server/respond.go` |
| `internal/httpapi/middleware.go` | `internal/infrastructure/server/middleware.go` (CORS, cache-control) |
| `internal/httpapi/artworks.go` | `internal/artwork/adapters/http/handlers.go` (split + reshaped) |
| `internal/httpapi/users.go` | `internal/user/adapters/http/handlers.go` |
| `internal/httpapi/tags.go` | `internal/artwork/adapters/http/tag_handlers.go` |
| `internal/httpapi/devseed.go` | `cmd/seeder/main.go` |
| `internal/db/pool.go` | `internal/infrastructure/database/connection.go` (rewritten for GORM) |
| `internal/db/migrate.go` | `internal/infrastructure/database/migrate.go` |
| `internal/dbtest/postgres.go` | `internal/infrastructure/testing/postgres.go` |

### 4.3 Dependency-direction invariant

```
adapters/http   adapters/postgres   adapters/oauth   ← may import: ports, domain, infrastructure
       ↓                ↓                  ↓
              ports                                   ← may import: domain (only)
                ↓
             service                                  ← may import: ports, domain
                ↓
             domain                                   ← may import: nothing internal
```

- **Banned import lists** (enforced by a CI Go AST checker):
  - `domain/` packages MUST NOT import any `internal/` path.
  - `ports/` packages MUST NOT import GORM, chi, zerolog, or any other framework.
  - `service/` packages MUST NOT import `gorm.io/...`, `github.com/go-chi/...`, or any adapter package.
- **Cross-slice rule:** no slice's `domain/`, `ports/`, or `service/` packages may import another slice's `domain/`, `ports/`, or `service/`. Cross-cutting needs go through `internal/infrastructure/`.

---

## 5. Test contract & coverage strategy

### 5.1 Two-phase timeline

```
Phase 0 — "Test net" (8 PRs, incremental, on master)
  PR 0.1   Test harness scaffolding
  PR 0.2   user        63.8% → 80%
  PR 0.3   storage     69.1% → 80%
  PR 0.4   image       69.7% → 80%
  PR 0.5   httpapi     69.2% → 80%
  PR 0.6   db + artwork top-up
  PR 0.7   Build the HTTP contract matrix (no goldens yet)
  PR 0.8   Lock the snapshots (capture goldens, activate as required CI)

Phase 1 — "Big bang" (single branch, single PR, internally a sequence of small commits)
  Merge gates: contract_suite_parity + coverage_threshold + wire_diff
              + banned_imports + smoke_test + schema_drift
```

### 5.2 Contract test matrix

For every endpoint in the current `internal/httpapi/router.go` **except `/dev/seed`** (see §5.2.1 for exclusion rationale), the suite runs the cartesian product of:

| Axis | Values |
|---|---|
| Auth state | anon, self-user, other-user |
| Resource state | empty, single, paginated, public/private mix |
| Request shape | minimal-valid, full-valid, boundary (max len, 0), malformed JSON, malformed multipart, wrong Content-Type |
| Expected status | 200, 201, 204, **302** (auth redirects), 400, 401, 403, 404, 409, **412** (image position taken), **415** (image bad content-type), 422, 500, **502** (oauth exchange failed) |

**Status-code provenance** (verified against current code):

| Status | Path | Source |
|---|---|---|
| 302 | `GET /auth/{provider}/start`, `GET /auth/{provider}/callback` | `auth/handlers.go` redirects |
| 412 | `POST /artworks/{id}/images` | `image/handler.go` `ErrPositionTaken` |
| 415 | `POST /artworks/{id}/images` | `image/handler.go` non-multipart Content-Type |
| 422 | `POST /artworks/{id}/images` | `image/handler.go` `ErrTooLarge`, `ErrContentTypeMismatch`, `decode_failed` |
| 502 | `GET /auth/{provider}/callback` | `auth/handlers.go` OAuth `exchange_failed` |

Resource states **exclude "soft-deleted"** because the schema does hard deletes (verified against `migrations/0001_init.up.sql` — no `deleted_at` columns; only `ON DELETE CASCADE` foreign keys).

#### 5.2.1 `/dev/seed` is excluded from the HTTP contract

The current router mounts `/dev/seed` only when `AppEnv == "test"` (`internal/httpapi/router.go:63-70`). It is a development/testing utility, not part of the product API surface that the frontend or external clients depend on.

- **PR 0.5** still adds direct tests for `/dev/seed` to lift `httpapi` package coverage to ≥ 80%, but those tests are not part of the byte-strict contract suite.
- **PR 0.7 / PR 0.8** do not snapshot `/dev/seed`.
- **Phase 1** is permitted to drop `/dev/seed` from the router (functionality moves to `cmd/seeder`).

This is the only documented gap between "every router endpoint" and "every contract endpoint."

Each test:
1. Boots a fresh `testcontainers/postgres` + `testcontainers/minio` instance.
2. Runs migrations (`golang-migrate`).
3. Seeds via `cmd/seeder` binary or `infrastructure/testing/fixtures/*.sql`.
4. Boots the API in-process via the helper `infrastructure/testing.BootApp(t)` with deterministic sources injected (see §5.3.2).
5. Issues an HTTP request, normalizes dynamic bytes (see §5.3.2), and snapshots:
   - Status code
   - Selected response headers (Content-Type, Cache-Control, Set-Cookie — with cookie value normalized)
   - Full JSON body (byte-strict, post-normalization)

```go
func assertGolden(t *testing.T, name string, got *http.Response) {
    body := normalizeDynamicBytes(got.Body)   // see §5.3.2
    // First run with GOLDEN_UPDATE=1: writes golden/<name>.json
    // Subsequent runs: byte-compare; diff fails the test
}
```

### 5.3 Snapshot strictness — byte-strict

After PR 0.8 captures snapshots, Phase 1 must reproduce them **exactly**, post-normalization (see §5.3.2). Encoding details that matter:

#### 5.3.1 Static encoding rules

- **JSON key order — alphabetical, not declaration order.** Current handlers build responses from `map[string]any{...}` and `map[string]string{...}` (e.g., `httpapi/render.go:25-30`, `httpapi/artworks.go:109`). Go's `encoding/json` marshals map keys in sorted order. Phase 1 DTO structs must declare fields **in alphabetical JSON-tag order** to reproduce current bytes. This is enforced via a CI lint pass (`json_field_order`) that parses each DTO struct and verifies tag-name sort.
- **Trailing newline — non-uniform across paths.** `internal/httpapi/render.go` uses `json.NewEncoder(w).Encode(...)` which emits a trailing `\n`. `internal/auth/handlers.go` uses `w.Write([]byte(rawJSON))` which does not. The contract captures both behaviors. Phase 1's `WriteError` and per-handler response writers must match the source path's current behavior — `auth/adapters/http/handlers.go` uses raw writes for the error paths the current `auth/handlers.go` does; everywhere else uses `Encode`.
- **Time formatting.** Current code uses `time.Time.UTC().Format(time.RFC3339)` for `created_at`/`published_at` (`httpapi/render.go:53,65`) and `time.RFC3339Nano` for cursor encoding (`httpapi/artworks.go:43,54`). Phase 1 mappers preserve both formats per field.
- **Nullable fields — keys are always present, values may be `null`.** Verified against `httpapi/render.go:19-31` (`renderUser`) and `httpapi/render.go:50-69` (`renderArtworkSummary`). The current code uses `map[string]any{...}` with `*string` or `*Time` *values*, so `nil` values render as JSON `null` and the key remains present in the output. Examples:
  - `avatar_url`: always present, `null` when `User.AvatarURL == ""` (`render.go:20-24,29`).
  - `published_at`: always present, `null` when `Artwork.PublishedAt == nil` (`render.go:51-55,64`).
  - `cover`: always present, `null` when no images attached (`render.go:56-58,66`).
  - `description`: included by `getArtworkHandler` only when non-empty (`artworks.go:269-272,288`) — explicit conditional, not `omitempty`.
  - `next_cursor`: present, `null` when no more pages (`encodeCursor` returns `*string`).
  Phase 1 DTOs must NOT use `,omitempty` on these fields. If using a struct with `*string` / `*time.Time`, the JSON tag should be plain `json:"avatar_url"` (no omitempty), so a nil value emits `"avatar_url": null` rather than dropping the key.
- **Two error shapes from §3** — `{"error":"x"}` and `{"error":"x","message":"y"}` — emitted from the appropriate paths.

#### 5.3.2 Dynamic-byte normalization

Several response bytes are non-deterministic in production (UUIDs, JWTs, signed-URL HMACs, random OAuth state, DB `now()` timestamps). The contract harness handles each via injection-where-possible-otherwise-normalization:

| Dynamic source | Where generated | Strategy |
|---|---|---|
| `time.Now` for JWT issuance | Go: `auth/jwt.go` (already injectable) | **Inject** — pass a fixed-clock provider in tests. |
| `time.Now` for signed URL expiry | Go: `auth/imgurl.go` (`URLBuilder`, already injectable) | **Inject** — same fixed clock as JWT. |
| Image upload IDs (`uuid.NewString()`) | Go: `image/service.go:84` | **Inject** — Phase 0 PR 0.7 introduces an `IDProvider` interface; production uses `uuid.NewString()`, tests use a deterministic counter. The single Go-side call site is switched in PR 0.7 as a no-op refactor. |
| OAuth state cookie value (`crypto/rand` → hex) | Go: `auth/handlers.go:123-127` (`randState`) | **Inject** — Phase 0 PR 0.7 makes `randState` accept a `RandReader` (defaulting to `crypto/rand.Reader`); tests inject a deterministic source. |
| User IDs, artwork IDs, image-row IDs, tag IDs, artwork_image IDs | **DB:** `gen_random_uuid()` as column DEFAULT (`migrations/0001_init.up.sql:3,15,31,48`). The INSERTs do not specify `id` — they rely on the DB default and `RETURNING id`. | **Normalize** — Go-side injection would require rewriting every INSERT to suppress the DB default (a behavior change forbidden by §1 non-goals). Snapshot normalizer regex-replaces UUID patterns (`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`) with `<UUID>` before compare. |
| DB `now()` timestamps in `created_at`/`updated_at`/`published_at` | DB: `now()` as column DEFAULT, plus explicit `now()` in artwork visibility flip | **Normalize** — Postgres `now()` is not easily injectable. Snapshot normalizer regex-replaces `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z` with `<TIMESTAMP>` before compare. |
| Signed URL HMAC suffix | Go: `auth/imgurl.go` (depends on per-env signing key) | **Normalize** — replace `\?sig=[0-9a-f]+&exp=\d+` with `?sig=<SIG>&exp=<EXP>`. |
| JWT cookie value | Go: `auth/jwt.go` (depends on per-env signing key) | **Normalize** — Set-Cookie header value for `auth=` is replaced with `auth=<JWT>` in normalized snapshots. |
| Cursor base64 strings | Go: `httpapi/artworks.go` (depends on the timestamp + UUID inside) | **Normalize** — captured normalized to `<CURSOR>` because they wrap normalized timestamps and UUIDs anyway. |

The normalizer is implemented once in `internal/infrastructure/testing/normalize.go` and applied uniformly. Snapshot golden files store the **post-normalization** bytes, so a Phase 1 implementation that emits a different UUID format would still fail (the normalizer would not match the new format).

#### 5.3.3 GORM compatibility

To keep GORM compatible with the locked snapshots:
- Pin GORM driver to its pgx-backed implementation (`gorm.io/driver/postgres` wraps pgx).
- Use explicit GORM type tags: `Type:"timestamp(6) with time zone"` etc., matching migrations.
- Forbid `db.AutoMigrate` (see §10).

### 5.4 Coverage scope per subpackage

| Subpackage type | Counted? | Test method |
|---|---|---|
| `<feature>/domain/` | yes (target ~100%) | unit tests, no deps |
| `<feature>/ports/` | **excluded** (interfaces only) | n/a |
| `<feature>/service/` | yes | unit tests with Mockery v3 mocks |
| `<feature>/adapters/postgres/` | yes (with `-tags=integration`) | testcontainers Postgres |
| `<feature>/adapters/http/` | yes | `httptest.NewServer` + service mocks |
| `<feature>/adapters/oauth/` | yes | mocked HTTP client |
| `<feature>/adapters/storage/` | yes | testcontainers minio |
| `cmd/api/` | excluded (wiring) | covered by smoke test |
| `internal/infrastructure/...` | excluded | covered transitively by contract suite |
| `pkg/signing/` | yes (target ~95%) | focused unit tests |

The 80% threshold is the **statement-weighted average across counted subpackages per slice**. CI command per slice (Phase 1 layout):

```bash
# Go test takes packages as space-separated args; -coverpkg takes a comma-separated list.
# Use two distinct variable shapes to avoid collapsing N packages into one comma-containing path.
TESTPKGS=$(go list ./internal/auth/... | grep -v /ports | grep -v /mocks)
COVERPKG=$(echo "$TESTPKGS" | tr '\n' ',' | sed 's/,$//')
go test -coverpkg="$COVERPKG" -coverprofile=auth.out -tags=integration $TESTPKGS
go tool cover -func=auth.out | tail -1
```

Both `/ports` (interfaces — no statements) and `/mocks` (generated code — would skew coverage) are excluded from **both** the `-coverpkg` instrumentation list and the test-run package list.

### 5.5 Mocks

Generated by Mockery v3. Config in repo root:

```yaml
# .mockery.yaml
all: false
dir: "{{.InterfaceDir}}/mocks"
outpkg: "{{.PackageName}}mocks"
packages:
  local/art-web/api/internal/auth/ports:    {}
  local/art-web/api/internal/user/ports:    {}
  local/art-web/api/internal/artwork/ports: {}
  local/art-web/api/internal/image/ports:   {}
  local/art-web/api/internal/infrastructure/storage: { interfaces: { Storage: {} } }
```

Mocks are committed to git under `<feature>/ports/mocks/`. CI runs `mockery --check` to verify they match the interface definitions. Mocks are used **only in service-layer unit tests**.

### 5.6 Bug-fix policy during Phase 0

Bugs discovered while writing Phase 0 tests are fixed in **separate, dedicated PRs** before the snapshot lock (PR 0.8). These PRs:

- Are labeled `phase-0-behavior-change`.
- Require an explicit reviewer signoff confirming the change is a bug fix, not a feature change.
- Land on master in sequence with the coverage-uplift PRs.

Bugs discovered *after* PR 0.8 lock ship as standalone hotfixes to master and are rebased into the Phase 1 branch.

---

## 6. Wire DI graph

### 6.1 Per-slice provider files

Each slice owns one `providers.go`:

```go
// internal/artwork/providers.go
package artwork

import (
    "github.com/google/wire"
    httpadapter "local/art-web/api/internal/artwork/adapters/http"
    "local/art-web/api/internal/artwork/adapters/postgres"
    "local/art-web/api/internal/artwork/ports"
    "local/art-web/api/internal/artwork/service"
)

var ProviderSet = wire.NewSet(
    postgres.NewArtworkRepo,
    postgres.NewTagRepo,
    service.NewArtworkService,
    service.NewVisibilityService,
    httpadapter.NewHandler,
    httpadapter.NewRouter,
    wire.Bind(new(ports.ArtworkRepository), new(*postgres.ArtworkRepo)),
    wire.Bind(new(ports.TagRepository),     new(*postgres.TagRepo)),
)
```

`infrastructure/storage` exposes its own `ProviderSet` (declared in `internal/infrastructure/storage/providers.go`) and is added to `wire.Build` in `cmd/api/wire.go` alongside the slice ProviderSets. Both `artwork` and `image` services receive the same `Storage` instance via Wire's standard binding rules. Slices import the `Storage` port type directly from `internal/infrastructure/storage` — this is the explicitly allowed cross-cutting infrastructure case (per §4.3).

### 6.2 Root injector

```go
//go:build wireinject
// cmd/api/wire.go
func InitializeApp(ctx context.Context, cfgPath string) (*App, func(), error) {
    wire.Build(
        config.ProviderSet,
        logger.ProviderSet,
        database.ProviderSet,         // *gorm.DB + Transactor + cleanup
        storage.ProviderSet,          // chosen by config: localfs vs r2
        auth.ProviderSet,
        user.ProviderSet,
        artwork.ProviderSet,
        image.ProviderSet,
        server.ProviderSet,           // composes *http.Server from RouteRegistrars
        wire.Struct(new(App), "*"),
    )
    return nil, nil, nil
}

type App struct {
    Server *http.Server
    DB     *gorm.DB
    Logger zerolog.Logger
}
```

### 6.3 Route composition

```go
// internal/infrastructure/server/registrar.go
type RouteRegistrar interface {
    RegisterRoutes(r chi.Router)
}

func ProvideRouteRegistrars(
    auth    *authhttp.Router,
    user    *userhttp.Router,
    artwork *artworkhttp.Router,
    image   *imagehttp.Router,
) []RouteRegistrar {
    return []RouteRegistrar{auth, user, artwork, image}
}
```

Adding a new slice means adding one parameter and one slice element here, plus adding the slice's `ProviderSet` to `wire.Build` in `cmd/api/wire.go`. No struct-fill magic.

### 6.4 Auth middleware sharing

`auth/adapters/http/middleware.go` exposes a concrete `*Middleware` struct with two methods:

```go
func (m *Middleware) ParseToken() func(http.Handler) http.Handler   // global
func (m *Middleware) RequireUser() func(http.Handler) http.Handler  // route gate
```

Other slices' `*Router` types accept `*authhttp.Middleware` as an injected dependency. Wire binds the same instance everywhere.

### 6.5 Cleanup chain

Providers needing teardown return `(T, func(), error)`:

```go
func NewConnection(cfg DatabaseConfig) (*gorm.DB, func(), error) {
    db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{...})
    if err != nil { return nil, nil, err }
    return db, func() {
        sqlDB, _ := db.DB()
        _ = sqlDB.Close()
    }, nil
}
```

Wire stitches cleanup functions into the returned `func()` from `InitializeApp`, in reverse construction order. Same pattern for `*http.Server` (graceful shutdown), log file outputs, and any future resource.

### 6.6 Generation workflow

```
make wire        cd cmd/api && go run github.com/google/wire/cmd/wire ./...
make verify      ensures wire_gen.go is up-to-date (CI gate)
make test        go test ./... -coverprofile=coverage.out
make contract    go test ./internal/infrastructure/testing/... -tags=contract
make mocks       go run github.com/vektra/mockery/v3 --config .mockery.yaml
```

CI gate:
```yaml
- name: verify wire is up-to-date
  run: |
    cd cmd/api && go run github.com/google/wire/cmd/wire ./...
    git diff --exit-code wire_gen.go
```

### 6.7 Config sectioning

```go
// internal/infrastructure/config/config.go
type AppConfig struct {
    Server   ServerConfig
    Database DatabaseConfig
    Auth     AuthConfig
    Storage  StorageConfig
    Image    ImageConfig
    Logger   LoggerConfig
}

// In config.ProviderSet:
wire.FieldsOf(new(*AppConfig), "Server", "Database", "Auth", "Storage", "Image", "Logger")
```

Each slice's provider receives only the sub-config it needs — `auth.NewService` takes `AuthConfig`, not `AppConfig`.

---

## 7. Domain & error-handling conventions

### 7.1 Error policy by layer

```
adapters/http   ← maps domain.Err* to HTTP status; logs once at the boundary;
                  emits the current API's error JSON shape (see §3 + §7.4)

      ↑
   service       ← returns domain sentinel errors; never logs;
                  wraps lower-level errors with fmt.Errorf("op: %w", err)

      ↑
adapters/postgres  ← translates gorm.ErrRecordNotFound → domain.ErrNotFound,
                     pgconn unique-violation → domain.ErrConflict;
                     never logs

      ↑
  domain         ← defines sentinel errors; pure
```

### 7.2 Domain sentinels

Each slice has `<feature>/domain/errors.go`. Sentinels are split into **HTTP-observable** (which the boundary maps to a status code visible to clients) and **internal-only** (which services *must* translate to a different sentinel before returning, to preserve current bytes).

```go
// internal/artwork/domain/errors.go
var (
    // HTTP-observable
    ErrNotFound         = errors.New("artwork: not found")            // → 404 not_found
    ErrNoImages         = errors.New("artwork: no images attached")   // → service-only, becomes 500/specific code

    // Internal-only — MUST be translated by the service before return
    ErrForbidden        = errors.New("artwork: caller does not own resource")
    ErrAlreadyPublished = errors.New("artwork: already published")
)
```

#### 7.2.1 Internal-only sentinel translation rules

The current API does NOT emit 403, and PATCH-with-current-visibility returns 204 (no error). To preserve those bytes, services translate internal sentinels at their public surface:

| Internal sentinel | Service-layer translation | Why |
|---|---|---|
| `domain.ErrForbidden` (caller doesn't own resource) | Service returns `domain.ErrNotFound` instead | Verified against `httpapi/artworks.go:174,236,254`: every non-owner access on a private/owned resource collapses to 404 (`err != nil \|\| a.UserID != uid` → `not_found`). The new code preserves this at the service boundary so handlers stay simple. |
| `domain.ErrAlreadyPublished` (Publish() called on already-public artwork) | `VisibilityService.Flip` returns `nil` (no-op) when target == current visibility | Verified against `internal/artwork/visibility.go:31-33` (early-return) and `httpapi/artworks.go:227` (handler writes 204 unconditionally after flip). The new code's domain method `Publish()` returns `ErrAlreadyPublished` for clarity inside the domain, but the visibility service catches it and returns nil to preserve the 204. |

**These sentinels MUST NOT appear in `WriteError`'s switch (§7.4).** A test verifies that no path maps to 403 or 409 in the contract suite.

### 7.3 Validation policy

- **DTO** (`adapters/http/dto.go`) validates **shape only**: presence, length, regex, range, enum. Produces 400 with the current API's error code. Uses `go-playground/validator/v10`.
- **Domain constructor** validates **cross-field invariants only**: things that can't be a single-field tag. Produces domain sentinel errors.
- **No overlap.** A single-field rule lives in exactly one place: the DTO.

#### 7.3.1 Phase 1 validation must mirror current behavior, not "ideal" behavior

The current handlers do **minimal** input validation. Verified against `internal/httpapi/artworks.go`:

| Endpoint | Currently validated | Currently NOT validated |
|---|---|---|
| `POST /artworks` (create) | `bad_json` (decode), `bad_visibility` (`oneof`), defaults `Visibility=""` to `"private"` | `Title` empty, `Title` length, `Tags` length/format, `Description` length |
| `PATCH /artworks/{id}` | `bad_json`, `bad_visibility`, `bad_cover_position` (`< 0`) | All field lengths, tag formats |
| `POST /artworks/{id}/images` | `unsupported_media_type`, `bad_multipart`, `manifest_required`, `file_count_mismatch` | (most edge cases via image service errors) |

The Phase 1 DTOs and validator config must **reproduce exactly this validation surface** — no stricter, no looser. Adding `required` or `max=200` to a field that today accepts any string would generate a 400 where today the request succeeds. That's a contract break.

Illustrative mapping (the actual validator tags are confirmed against PR 0.8 snapshots):

```go
// internal/artwork/adapters/http/dto.go — matches current behavior
type CreateArtworkRequest struct {
    Description string   `json:"description"`                               // no validation
    Tags        []string `json:"tags"`                                      // no validation
    Title       string   `json:"title"`                                     // no validation
    Visibility  string   `json:"visibility" validate:"omitempty,oneof=public private"`  // omitempty preserves the empty-string default-to-private behavior
}
// Note: alphabetical declaration order (per §5.3.1).
// "Visibility = '' → defaults to 'private'" is handled in the handler post-validation,
// matching httpapi/artworks.go:150-152 today.
```

If the team later wants stricter validation, that ships as a follow-up `phase-0-behavior-change` PR (per §5.6) — captured by snapshots, then locked. **It is not part of this refactor's scope.**

#### 7.3.2 Domain invariants are unchanged in scope

```go
func (a *Artwork) Publish() error {
    if a.Visibility == VisibilityPublic { return ErrAlreadyPublished }
    if len(a.Images) == 0                { return ErrNoImages }
    a.Visibility = VisibilityPublic
    return nil
}
```

Domain invariants are NEW logic (the current code has no centralized "publish" rule), but they sit **inside** the existing flip path. `Publish()` is called by `service.VisibilityService` which today is `internal/artwork/visibility.go`. The current code has no equivalent of `ErrAlreadyPublished` — it returns `nil` early when the visibility already matches (`visibility.go:31-33`). Phase 1 must preserve that no-op behavior at the HTTP boundary: a `PATCH` with `visibility=current_value` returns 204 today and must return 204 in Phase 1, even though the new domain method returns `ErrAlreadyPublished`. The service translates `ErrAlreadyPublished` to a no-op 204 to match.

### 7.4 HTTP error mapping — locked to current bytes

```go
// internal/infrastructure/server/errors.go

// HTTPError matches the current API's two distinct shapes (see §3):
//   { "error": "<code>" }                          — most paths
//   { "error": "<code>", "message": "<detail>" }   — parse/cursor errors
type HTTPError struct {
    Error   string `json:"error"`
    Message string `json:"message,omitempty"`
}

func WriteError(w http.ResponseWriter, log zerolog.Logger, err error) {
    // NOTE: domain.ErrForbidden and domain.ErrAlreadyPublished are NOT in this
    // switch. Per §7.2.1 they are internal-only; services translate them to
    // ErrNotFound or no-op (nil) before returning. A contract-suite test asserts
    // no path emits 403 or 409 (other than image upload's 409 fingerprint_mismatch
    // which is its own image-layer code, not a domain error).
    switch {
    case errors.Is(err, domain.ErrNotFound):
        write(w, 404, HTTPError{Error: "not_found"})
    case errors.As(err, &validator.ValidationErrors{}):
        write(w, 400, validationToHTTPError(err))      // see §7.5
    case errors.Is(err, errBadJSON):                          // http-layer parse error
        write(w, 400, HTTPError{Error: "bad_json"})
    case errors.Is(err, errBadCursor):                         // http-layer parse error
        write(w, 400, HTTPError{Error: "bad_cursor", Message: err.Error()})
    // ... and so on for every code in §3 — note that several codes (e.g.,
    // create_failed, patch_failed, flip_failed) are operation-name leaks that
    // get emitted on opaque 500s. The mapping uses operation context from the
    // wrapped error to decide which code to emit, matching current behavior.
    default:
        log.Error().Err(err).Msg("internal error")
        write(w, 500, HTTPError{Error: "internal_error"})    // confirm exact code via PR 0.8 snapshot
    }
}
```

The full mapping table is built during PR 0.8 (snapshot lock) by inspecting every error code emitted in the current handlers. Every code in §3 must be reproduced.

### 7.5 Validator-error translator

`go-playground/validator/v10`'s default error messages do not match the current API's error wording. Phase 1 includes a translator:

```go
// internal/infrastructure/server/validation.go
func validationToHTTPError(verrs validator.ValidationErrors) HTTPError {
    // Current code does not expose field-level validation details — most validation
    // failures collapse to a single error code (e.g., "bad_visibility", "bad_json").
    // The translator picks the appropriate code based on which field/tag failed,
    // matching the codes the original handlers emitted.
    fe := verrs[0]  // current API surfaces only the first failure
    switch {
    case fe.Field() == "Visibility" && fe.Tag() == "oneof":
        return HTTPError{Error: "bad_visibility"}
    // The full table is built during PR 0.8 from captured snapshots.
    // Note: per §7.3.1 the current API does NOT validate Title/Tags/Description
    // length or format, so there is no `bad_title`, `bad_tags`, etc. — those would
    // be new error codes and are explicitly out of scope for this refactor.
    default:
        return HTTPError{Error: "bad_json"}
    }
}
```

### 7.6 Transactions — implicit context-key via `Transactor`

```go
// internal/infrastructure/database/transactor.go
type ctxKey struct{}

type Transactor struct{ db *gorm.DB }

func (t *Transactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
    return t.db.Transaction(func(tx *gorm.DB) error {
        return fn(context.WithValue(ctx, ctxKey{}, tx))
    })
}

func DB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
    if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok { return tx }
    return fallback
}
```

Every repo method calls `database.DB(ctx, r.db)` to get the active GORM handle (transactional or not).

#### 7.6.1 The visibility-flip flow — preserves current "storage-first, DB-within-tx, compensate-on-failure" semantics

Verified against current `internal/artwork/visibility.go:23-93`. The current code's order is:

```
1. Read images (artwork_images rows) — outside any transaction
2. Move ALL files in storage to the new visibility prefix
   - Track `completed []flipMove` as each succeeds
   - On any move failure: rollback completed moves (best-effort), return error.
     No DB writes have happened yet.
3. Begin DB transaction
4. UPDATE artwork_images SET storage_key = new_path  (within tx)
5. UPDATE artworks SET visibility = target          (within tx)
6. COMMIT
   - On any DB error or commit failure: rollback storage moves (best-effort), return error.
7. RollbackLog fires only when a *compensating* storage rollback move itself fails
   (i.e., we tried to undo and the undo failed — a leak).
```

The Phase 1 service preserves this exact ordering. **DB-tx-first is incompatible with "no observable side effects"** because a successful DB commit followed by a failed storage move would leak a published artwork with files still in the private prefix (a state the current code never produces).

```go
// internal/artwork/service/visibility.go
func (s *VisibilityService) Flip(ctx context.Context, artworkID string, target string) error {
    if target != "public" && target != "private" { return ErrBadTarget }

    art, err := s.arts.Get(ctx, artworkID)
    if err != nil { return err }
    if art.Visibility == target { return nil }   // matches current early-return at visibility.go:31-33

    // Step 1 + 2: read images, move storage. NO DB tx open yet.
    moves, err := s.arts.PendingFlipMoves(ctx, artworkID, art.Visibility, target)
    if err != nil { return err }
    completed, err := s.movesApply(ctx, moves)
    if err != nil {
        s.movesRollback(ctx, completed)   // best-effort
        return err
    }

    // Step 3-6: DB tx that updates artwork_images.storage_key + artworks.visibility.
    err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
        if err := s.arts.UpdateImageStorageKeys(ctx, completed); err != nil { return err }
        return s.arts.SetVisibility(ctx, artworkID, target)
    })
    if err != nil {
        s.movesRollback(ctx, completed)   // best-effort; logs each failed undo via RollbackReporter
        return err
    }
    return nil
}

func (s *VisibilityService) movesRollback(ctx context.Context, completed []flipMove) {
    for i := len(completed) - 1; i >= 0; i-- {
        m := completed[i]
        if err := s.storage.Move(ctx, m.dst, m.src); err != nil {
            s.rollback.Report(ctx, m.id, err)   // matches current RollbackLog semantics:
                                                // fired only when the *undo* itself fails (a leak)
        }
    }
}
```

#### 7.6.2 Other multi-repo transactions

For flows that are purely DB-bound (e.g., create-artwork-with-tags), the `WithinTx` pattern is straightforward — no storage involvement, no compensating actions:

```go
func (s *ArtworkService) Create(ctx context.Context, uid string, req CreateInput) (string, error) {
    var id string
    err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
        var err error
        id, err = s.arts.Insert(ctx, uid, req)
        if err != nil { return err }
        if len(req.Tags) > 0 {
            return s.tags.SetTags(ctx, id, req.Tags)
        }
        return nil
    })
    return id, err
}
```

### 7.7 RollbackReporter (preserves `artwork.RollbackLog`)

The current code uses a package-level global `artwork.RollbackLog = func(err error) { ... }` (`internal/artwork/visibility.go:104`, set in `cmd/api/main.go:74`). It fires **only when a compensating storage-rollback move itself fails** — i.e., the primary op failed, we tried to undo, and the undo failed (a leak). It does NOT fire on the primary op's failure.

Phase 1 replaces this with a Wire-injected interface preserving the same fire condition:

```go
// internal/artwork/ports/rollback.go
type RollbackReporter interface {
    // Report is invoked when a compensating storage-rollback move fails after a
    // primary operation has already failed. The artwork may now have storage
    // objects in an inconsistent location relative to its DB state.
    Report(ctx context.Context, imageID string, err error)
}

// internal/artwork/adapters/log/rollback.go — default implementation
func (r *zerologRollbackReporter) Report(ctx context.Context, imageID string, err error) {
    r.log.Error().Err(err).Str("image_id", imageID).Msg("flip rollback")
}
```

A contract test triggers this path (mocked storage `Move` returns ok on forward direction, fails on reverse direction; DB tx then fails to commit; rollback fires for each completed forward move) and asserts the reporter fires once per image with the same fields today's code logs.

### 7.8 Mappers — pure on already-loaded models

```go
// internal/artwork/adapters/postgres/repo.go
func (r *ArtworkRepo) GetWithDetails(ctx context.Context, id uuid.UUID) (*domain.Artwork, error) {
    var m artworkModel
    if err := database.DB(ctx, r.db).WithContext(ctx).
        Preload("Tags").Preload("Images").
        First(&m, "id = ?", id).Error; err != nil { ... }
    return mappers.ToDomainArtwork(&m), nil
}

// internal/artwork/adapters/postgres/mappers.go — pure
func ToDomainArtwork(m *artworkModel) *domain.Artwork { /* field copies only */ }
```

Each repo method documents which associations it preloads. Mappers panic (programmer error, not runtime error) if they encounter `nil` associations they expected.

### 7.9 Logging convention (zerolog)

| Layer | Logs? | What |
|---|---|---|
| `domain/` | never | — |
| `service/` | never | wraps errors with `op` context |
| `adapters/postgres/` | never | translates DB errors only |
| `adapters/http/` | once, at WriteError | full wrapped error chain |
| `infrastructure/...` | startup/shutdown only | never request-scoped |

Each error appears in logs **exactly once**. A grep for `log/slog` after Phase 1 must return zero matches.

---

## 8. Phase 0 work breakdown

### 8.1 PR-by-PR

| PR | Scope | Acceptance criteria |
|---|---|---|
| **0.1** Test harness | Move `internal/dbtest` → `internal/infrastructure/testing`. Add testcontainers + minio harness, `BootApp(t)` helper, golden-file machinery, `GOLDEN_UPDATE=1` regen mode. Add CI job `contract_suite` (skipped initially). | All current tests pass; harness compiles; harness self-test boots current API and asserts `GET /healthz` returns 200. |
| **0.2** `user` 63.8 → 80 | Tests for the actual surface area of `internal/user/repo.go`: `slugify` edge cases (empty input → `"user"`, length truncation at 32, non-ASCII characters), slug-collision retry path (`UpsertOAuth` loop in `repo.go:59-83`), slug-exhaustion error after 50 attempts, `oauth_provider+oauth_subject` unique-constraint re-read path (`repo.go:73-79`), `Get(404) → ErrNotFound` mapping, `GetBySlug` non-existent-slug pass-through, non-`23505` Postgres error pass-through (`uniqueConstraint`). **Excludes:** profile-update tests (no PATCH route exists), soft-delete tests (no `deleted_at` column — schema has hard deletes only). | `go test ./internal/user/... -cover` ≥ 80%. |
| **0.3** `storage` 69.1 → 80 | Tests for: r2 multi-part edge cases, localfs path-traversal guard, missing-bucket error, content-type round-trip. | `go test ./internal/storage/... -cover` ≥ 80%. |
| **0.4** `image` 69.7 → 80 | Tests for: orphan cleanup race, content-type mismatch, signed-URL expiry boundary, blurhash error path. | `go test ./internal/image/... -cover` ≥ 80%. |
| **0.5** `httpapi` 69.2 → 80 | Tests for: privacy matrix completeness, devseed gated routes (coverage only — `/dev/seed` is not part of the byte-strict contract per §5.2.1), error responses for every 4xx path including 412/415/422 from image upload. | `go test ./internal/httpapi/... -cover` ≥ 80%. |
| **0.6** `db` + `artwork` top-up | `artwork` 78.3 → 80 (visibility transition edge cases) — required for the slice's 80% gate. `db` 75 → 80 is opportunistic (infrastructure is excluded from the gate per §5.4) but worth doing while the harness work is fresh. | `artwork` ≥ 80%; `db` ≥ 80% best-effort. |
| **0.7** Build the HTTP contract matrix + deterministic injection | Add the cartesian-matrix test file under `internal/infrastructure/testing/contract/`. Add `IDProvider` interface and switch existing UUID call sites to use it (no-op refactor — production uses `uuid.New()`, tests use a deterministic counter). Add normalizer (`internal/infrastructure/testing/normalize.go`) per §5.3.2. **No goldens yet.** Tests fail without snapshots — that's expected. | Reviewers focus on (a) matrix completeness, (b) `IDProvider` adoption is a no-op, (c) normalizer rules are correct. |
| **0.8** Lock the snapshots | Run with `GOLDEN_UPDATE=1`; commit `golden/*.json`; activate `contract_suite` as a required CI check; add CODEOWNERS protection on `golden/`. Build the validator-translator table (§7.5) and confirm validator-config table (§7.3.1) by inspecting captured bytes. Confirm `/dev/seed` is excluded from the snapshot set per §5.2.1. | `contract_suite` passes; `golden/` is non-empty; CODEOWNERS blocks unauthorized changes; `/dev/seed` snapshots are absent. |

Bug fixes between coverage uplifts and PR 0.8 lock land as separate `phase-0-behavior-change` PRs (see §5.6).

### 8.2 Coverage measurement: pre-Phase-1 vs post-Phase-1

Coverage is measured against different package paths in each phase, because Phase 0 lands on the *old* layout and Phase 1 introduces the *new* one:

- **Phase 0** measures the old, flat package paths: `internal/auth`, `internal/user`, `internal/artwork`, `internal/image`, `internal/httpapi`, `internal/storage`, `internal/db`. The 80% gate applies to the four business slices: `auth`, `user`, `artwork`, `image`.
- **Phase 1** measures the new, layered subpackages per §5.4. The 80% gate applies to the four business slices' counted subpackages, statement-weighted.

The CI threshold check uses different `go list` arguments before and after the layout-move commit. Both are encoded in the `coverage_threshold` job.

### 8.3 Phase 0 hard gates (block Phase 1 from starting)

Before the Phase 1 branch is created:

- [ ] PR 0.8 merged on master.
- [ ] `go test ./... -coverprofile=c.out` shows all four business slices ≥ 80% on the old package paths (per §8.2).
- [ ] `contract_suite` CI job exists, is required, and is currently green on master.
- [ ] `golden/` directory exists, is non-empty, and has CODEOWNERS protection.
- [ ] No frontend changes planned during Phase 1's branch lifetime, or coordination owner identified.
- [ ] Branch protection on master forbids force-push.

---

## 9. Phase 1 — single-branch big-bang

### 9.1 Commit-ordering constraints (mandatory)

The branch is a sequence of small, independently-buildable commits. Exact boundaries are at the implementer's discretion, subject to:

- **C1** Layout moves precede behavioral changes. Commits tagged `refactor(layout)` may not change function bodies.
- **C2** The pgx → GORM swap is the **last commit before cleanup**. Every prior commit builds and passes tests on pgx.
- **C3** Each commit individually leaves `contract_suite` green. Reviewers can `git checkout <any-commit>` and the suite passes.
- **C4** No commit may both move files AND change driver/library semantics.

### 9.2 Reasonable commit order (guide, not contract)

```
deps          add gorm, wire, zerolog, validator (go.mod only)
layout-move   files move to new tree (still pgx, slog, manual DI;
              postgres adapter dirs hold pgx-backed repos temporarily)
infra-base    add internal/infrastructure/{config,logger,server,testing}
wire          introduce wire.go + wire_gen.go in cmd/api
logger        slog → zerolog
domain        introduce domain/ packages per slice (entities, errors)
ports         introduce ports/ packages, services accept interface deps
gorm          THE BIG ONE — pgx → GORM in adapters/postgres,
              including new gorm_models + mappers.go (pure functions)
http-errors   single WriteError boundary; validator translator
seeder        cmd/seeder/main.go extracted; router drops /dev/seed
cleanup       remove dead code; final go vet + golangci-lint
```

Mappers ship in the GORM commit because they map between domain types and GORM model structs — they have no meaningful form before the GORM commit lands.

### 9.3 Branch hygiene

- **Weekly rebase against master** is mandatory.
- **Conflict ceiling:** if a single rebase produces > 100 lines of conflict, pause Phase 1 and resolve in a dedicated PR before continuing.
- **Snapshot updates forbidden** during Phase 1 unless correcting a snapshot that PR 0.8 captured incorrectly. Such corrections require a hotfix PR to master with reviewer signoff, then rebase into Phase 1.

---

## 10. Merge gates & CI checks

### 10.1 Required CI jobs for Phase 1 PR

| Job | Check | Failure mode |
|---|---|---|
| `contract_suite_parity` | Every snapshot byte-equivalent to PR 0.8's locked set | Diff printed; fix the new code, not the snapshot |
| `coverage_threshold` | `auth/`, `user/`, `artwork/`, `image/` ≥ 80% statement coverage (per §5.4) | Show missing lines |
| `wire_diff` | `cd cmd/api && wire ./...` produces zero diff against committed `wire_gen.go` | Re-run `make wire` and commit |
| `banned_imports` | AST checker: `domain/` imports nothing internal; `ports/` and `service/` import no framework | List the offending imports |
| `schema_drift` | Diff GORM-generated schema against live post-migrate schema | Adjust GORM tags; never `db.AutoMigrate` |
| `smoke_test` | `cmd/api/main_test.go` boots `InitializeApp`, makes one request per slice, shuts down | Boot/runtime error |
| `existing_unit_tests` | Re-run after each commit, not just at HEAD | Bisect to offending commit |
| `mockery_check` | `mockery --check` confirms mocks match interfaces | Re-run `make mocks` and commit |
| `json_field_order` | DTO struct fields are declared in alphabetical JSON-tag order (per §5.3.1) | Re-order fields; matches current map-based output |

### 10.2 GORM AutoMigrate is forbidden

The Phase 1 implementation **MUST NOT** call `db.AutoMigrate`. Schema is owned exclusively by `migrations/*.sql` (golang-migrate). GORM struct tags must declare types that match the existing schema exactly — they document GORM's marshaling behavior, not the source of truth.

The `schema_drift` CI job runs:
```bash
# Apply migrations
golang-migrate up
# Generate schema GORM would produce
go run ./internal/infrastructure/database/cmd/dump_gorm_schema > gorm.sql
# Diff against live schema
pg_dump --schema-only > live.sql
diff gorm.sql live.sql   # any diff fails
```

### 10.3 Smoke test contents

```go
// cmd/api/main_test.go
func TestMainBootsCleanly(t *testing.T) {
    cfg := testconfig.New()
    app, cleanup, err := InitializeApp(context.Background(), cfg)
    require.NoError(t, err)
    defer cleanup()

    // Verify each slice's routes are registered (status != 405 and != "page not found")
    for _, probe := range []routeProbe{
        {method: "GET",  path: "/artworks"},
        {method: "GET",  path: "/users/probe-slug"},
        {method: "GET",  path: "/auth/google/start"},
        {method: "GET",  path: "/healthz"},
    } {
        resp := makeRequest(app.Server, probe.method, probe.path)
        require.NotEqual(t, http.StatusMethodNotAllowed, resp.StatusCode, probe)
        require.NotContains(t, string(resp.Body), "page not found", probe)
    }
}
```

---

## 11. Post-merge timeline & fix-forward policy

| Window | Strategy | Master merge freeze |
|---|---|---|
| **T+0 to T+48h** | Pre-written `revert/refactor-go-scaffolding` PR is viable | Yes — only Phase-1-fix-forward PRs allowed; branch protection enforces |
| **T+48h to T+7d** | Fix-forward primary; revert is backup but requires conflict cleanup | Partial — non-trivial PRs queued; trivial fixes allowed |
| **After T+7d** | Fix-forward only | Lifted |

The 48-hour freeze is the spec's most operationally expensive commitment. Team-wide acknowledgement is required before Phase 1 branch is created.

**Canary requirements:**

1. Phase 1 merged commit runs on staging for ≥ 48 hours before production promotion.
2. First production deploy lands on a Tuesday morning (max time before weekend if rollback is needed).
3. Monitoring (existing Grafana / Sentry / equivalent) is reviewed daily during the canary window.

---

## 12. Out-of-scope (the YAGNI list)

The following are explicitly **out of scope** for this refactor and become follow-up PRs after Phase 1 lands:

- ✗ Adding new endpoints
- ✗ Changing API request/response shapes (including error shapes)
- ✗ Cleaning up internal-name leaks in error codes (`*_failed`)
- ✗ Changing migrations beyond what GORM tags require
- ✗ Adding new feature slices (`comments`, `likes`, etc.)
- ✗ Performance optimization (unless required by snapshot diffs)
- ✗ Test-style refactors not required by the layout move
- ✗ Documentation rewrites beyond updating module-level comments
- ✗ Making `cmd/seeder` production-grade (it's a dev tool; same scope as today)

---

## 13. Open questions (pre-implementation)

These do not block plan generation but should be resolved before Phase 1 begins:

1. **Mockery v3 vs v2:** the reference uses v3 syntax. Confirm v3 is acceptable in CI runners.
2. **CODEOWNERS for `golden/`:** which GitHub users/teams should be the required reviewers?
3. **48-hour merge freeze:** does the team have a defined "freeze announcement" channel and process?
4. **Validation translator table:** how exhaustive should it be? Spec says "every code observed in the current code"; PR 0.8 will confirm the full list.
5. **Wire library version pin:** the reference uses `github.com/google/wire` (no version specified). We pin to the latest stable at Phase 1 branch creation time.

---

## 14. References

- Reference scaffolding: [github.com/cuongtranba/go-scaffolding](https://github.com/cuongtranba/go-scaffolding)
- Brainstorming session conversation: 2026-05-04
- Current product context: `PRODUCT.md`
- Current backend overview: `README.md`
- Wire docs: [github.com/google/wire](https://github.com/google/wire)
- GORM docs: [gorm.io](https://gorm.io)
- Mockery v3: [vektra/mockery](https://github.com/vektra/mockery)
- zerolog: [rs/zerolog](https://github.com/rs/zerolog)
- validator/v10: [go-playground/validator](https://github.com/go-playground/validator)
