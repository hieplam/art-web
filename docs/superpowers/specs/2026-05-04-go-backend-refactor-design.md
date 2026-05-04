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

### Current error response shape (locked from `internal/httpapi/`)

The current API emits errors as JSON in two distinct shapes. Both are part of the contract:

```json
// Shape A — most paths
{ "error": "<snake_case_code>" }

// Shape B — parse/cursor errors with detail
{ "error": "<snake_case_code>", "message": "<original error string>" }
```

Content-Type: `application/json; charset=utf-8`. Observed code values include:
`unauthorized`, `not_found`, `bad_json`, `bad_cursor`, `bad_visibility`, `bad_cover_position`,
`create_failed`, `patch_failed`, `flip_failed`, `tag_failed`, `delete_failed`,
`list_failed`, `user_lookup_failed`. The Phase 1 implementation reproduces these exactly.

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

For every endpoint in the current `internal/httpapi/router.go`, the suite runs the cartesian product of:

| Axis | Values |
|---|---|
| Auth state | anon, self-user, other-user |
| Resource state | empty, single, paginated, public/private mix, soft-deleted |
| Request shape | minimal-valid, full-valid, boundary (max len, 0), malformed JSON |
| Expected status | 200, 201, 204, 400, 401, 403, 404, 409, 422 |

Each test:
1. Boots a fresh `testcontainers/postgres` + `testcontainers/minio` instance.
2. Runs migrations (`golang-migrate`).
3. Seeds via `cmd/seeder` binary or `infrastructure/testing/fixtures/*.sql`.
4. Boots the API in-process via the helper `infrastructure/testing.BootApp(t)`.
5. Issues an HTTP request and snapshots:
   - Status code
   - Selected response headers (Content-Type, Cache-Control, Set-Cookie)
   - Full JSON body (byte-strict)

```go
func assertGolden(t *testing.T, name string, got *http.Response) {
    // First run with GOLDEN_UPDATE=1: writes golden/<name>.json
    // Subsequent runs: byte-compare; diff fails the test
}
```

### 5.3 Snapshot strictness — byte-strict

After PR 0.8 captures snapshots, Phase 1 must reproduce them **exactly**, including:
- `time.Time` formatted as RFC3339 with UTC offset
- Nullable fields rendered consistently (current code uses `*string` + `omitempty` for some, explicit `null` for others — capture as-is)
- JSON key order (Go's `encoding/json` preserves declaration order — DTOs must keep field order)
- The two error shapes from §3 — `{"error":"x"}` and `{"error":"x","message":"y"}` — emitted from the appropriate paths

To keep GORM compatible:
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

The 80% threshold is the **statement-weighted average across counted subpackages per slice**. CI command per slice:

```bash
go test -coverpkg=./internal/auth/... -coverprofile=auth.out -tags=integration \
        $(go list ./internal/auth/... | grep -v /ports)
go tool cover -func=auth.out | tail -1
```

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

Each slice has `<feature>/domain/errors.go`:

```go
// internal/artwork/domain/errors.go
var (
    ErrNotFound         = errors.New("artwork: not found")
    ErrForbidden        = errors.New("artwork: caller does not own resource")
    ErrAlreadyPublished = errors.New("artwork: already published")
    ErrNoImages         = errors.New("artwork: no images attached")
)
```

### 7.3 Validation policy

- **DTO** (`adapters/http/dto.go`) validates **shape only**: presence, length, regex, range, enum. Produces 400 with field details. Uses `go-playground/validator/v10`.
- **Domain constructor** validates **cross-field invariants only**: things that can't be a single-field tag. Produces domain sentinel errors.
- **No overlap.** A single-field rule lives in exactly one place: the DTO.

```go
type CreateArtworkRequest struct {
    Title      string   `json:"title"      validate:"required,min=1,max=200"`
    Tags       []string `json:"tags"       validate:"max=20,dive,min=2,max=40,alphanumdash"`
    Visibility string   `json:"visibility" validate:"required,oneof=public private"`
}

func (a *Artwork) Publish() error {
    if a.Visibility == VisibilityPublic { return ErrAlreadyPublished }
    if len(a.Images) == 0                { return ErrNoImages }
    a.Visibility = VisibilityPublic
    return nil
}
```

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
    switch {
    case errors.Is(err, domain.ErrNotFound):
        write(w, 404, HTTPError{Error: "not_found"})
    case errors.Is(err, domain.ErrForbidden):
        write(w, 403, HTTPError{Error: "forbidden"})
    case errors.Is(err, domain.ErrAlreadyPublished):
        write(w, 409, HTTPError{Error: "already_published"})
    case errors.As(err, &validator.ValidationErrors{}):
        write(w, 400, validationToHTTPError(err))      // see §7.5
    case errors.Is(err, errBadJSON):                          // http-layer parse error
        write(w, 400, HTTPError{Error: "bad_json"})
    case errors.Is(err, errBadCursor):                         // http-layer parse error
        write(w, 400, HTTPError{Error: "bad_cursor", Message: err.Error()})
    // ... and so on for every code observed in the current code
    default:
        log.Error().Err(err).Msg("internal error")
        write(w, 500, HTTPError{Error: "internal"})    // matches current 500 fallback
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
    case fe.Field() == "Title":
        return HTTPError{Error: "bad_title"}        // verify against snapshot
    // ... full table built during PR 0.8
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

Every repo method calls `database.DB(ctx, r.db)` to get the active GORM handle (transactional or not). Services compose multiple repos in a single `WithinTx`:

```go
func (s *VisibilityService) Publish(ctx context.Context, id uuid.UUID) error {
    var art *domain.Artwork
    err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
        var err error
        if art, err = s.arts.Get(ctx, id); err != nil       { return err }
        if err = art.Publish(); err != nil                  { return err }
        if err = s.arts.Save(ctx, art); err != nil          { return err }
        return s.tags.IncrementUsage(ctx, art.Tags)
    })
    if err != nil { return err }

    // Outside DB tx — storage is a separate consistency domain.
    if err := s.storage.Move(ctx, art.PrivatePath(), art.PublicPath()); err != nil {
        s.rollback.Report(ctx, art.ID, err)   // matches current artwork.RollbackLog contract
        return err
    }
    return nil
}
```

### 7.7 RollbackReporter (preserves `artwork.RollbackLog`)

The current code uses a global `artwork.RollbackLog = func(err error) { log.Error(...) }` set in `cmd/api/main.go`. Phase 1 replaces this with a Wire-injected interface:

```go
// internal/artwork/ports/rollback.go
type RollbackReporter interface {
    Report(ctx context.Context, artworkID uuid.UUID, err error)
}

// Default implementation in adapters/log:
func (r *zerologRollbackReporter) Report(ctx context.Context, id uuid.UUID, err error) {
    r.log.Error().Err(err).Str("artwork_id", id.String()).Msg("flip rollback")
}
```

A contract test triggers this path (mocked storage failure post-commit) and asserts the artwork ends in the inconsistent-but-recoverable state today's code produces.

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
| **0.2** `user` 63.8 → 80 | Tests for: slug uniqueness conflict, `Get(404)`, profile update no-fields, soft-delete read. | `go test ./internal/user/... -cover` ≥ 80%. |
| **0.3** `storage` 69.1 → 80 | Tests for: r2 multi-part edge cases, localfs path-traversal guard, missing-bucket error, content-type round-trip. | `go test ./internal/storage/... -cover` ≥ 80%. |
| **0.4** `image` 69.7 → 80 | Tests for: orphan cleanup race, content-type mismatch, signed-URL expiry boundary, blurhash error path. | `go test ./internal/image/... -cover` ≥ 80%. |
| **0.5** `httpapi` 69.2 → 80 | Tests for: privacy matrix completeness, devseed gated routes, error responses for every 4xx path. | `go test ./internal/httpapi/... -cover` ≥ 80%. |
| **0.6** `db` + `artwork` top-up | `artwork` 78.3 → 80 (visibility transition edge cases) — required for the slice's 80% gate. `db` 75 → 80 is opportunistic (infrastructure is excluded from the gate per §5.4) but worth doing while the harness work is fresh. | `artwork` ≥ 80%; `db` ≥ 80% best-effort. |
| **0.7** Build the HTTP contract matrix | Add the cartesian-matrix test file under `internal/infrastructure/testing/contract/`. **No goldens yet.** Tests fail without snapshots — that's expected. | Reviewers focus on whether the matrix is *complete and correct*. |
| **0.8** Lock the snapshots | Run with `GOLDEN_UPDATE=1`; commit `golden/*.json`; activate `contract_suite` as a required CI check; add CODEOWNERS protection on `golden/`. Build the validation translator table by inspecting captured bytes. | `contract_suite` passes; `golden/` is non-empty; CODEOWNERS blocks unauthorized changes. |

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
