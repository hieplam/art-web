# Go backend refactor — Phase 1 implementation plan ("Big bang")

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the feature-sliced hexagonal layout + Wire DI + GORM + zerolog + validator/v10 in `api/internal/`, on a single branch, in 11 strict-ordered commits, with the byte-strict HTTP contract suite green at every commit.

**Architecture:** 4 business slices (`auth`, `user`, `artwork`, `image`) — each with `domain/`, `ports/`, `service/`, `adapters/{http,postgres,oauth}/` — plus `internal/infrastructure/` for cross-cutting concerns (`config`, `database`, `logger`, `server`, `storage`, `health`, `testing`). All ports are interfaces in pure-Go packages; services accept interfaces; adapters import services and ports; domain imports nothing internal. CI enforces the dependency direction via an AST checker.

**Tech Stack:**
- `gorm.io/gorm` + `gorm.io/driver/postgres` (replaces `jackc/pgx/v5` directly used; the driver still uses pgx underneath)
- `github.com/google/wire` (compile-time DI; replaces manual wiring in `cmd/api/main.go`)
- `github.com/rs/zerolog` (replaces `log/slog`)
- `github.com/go-playground/validator/v10` (DTO shape validation; produces 400 with current-API codes)
- `github.com/vektra/mockery/v3` (mock generation; mocks committed to git under `<slice>/ports/mocks/`)
- `golang-migrate/migrate/v4` — **unchanged** (GORM uses the same SQL files; `db.AutoMigrate` is forbidden by spec §10.2)
- `go-chi/chi/v5` — **unchanged** (router stays the same)

**Spec reference:** `docs/superpowers/specs/2026-05-04-go-backend-refactor-design.md`. When this plan and the spec disagree, the spec wins — file an issue and update the plan.

**Phase 0 inputs (now on master):**
- `api/internal/httpapi/contract/golden/*.json` — 56 byte-strict snapshots (the canary).
- `api/internal/httpapi/contract/error_codes_observed.md` — canonical (status, code) pairs for the `WriteError` switch.
- 4 business slices ≥ 80% coverage; `make test-cover-slices` is the gate.
- Test seams already shipped: `image.IDProvider` / `image.CounterIDProvider`, `auth.RandReader` / `auth.SetStateRandForTest`, `?suffix=` on `/dev/seed`, `dbtest.BootApp` / `dbtest.AssertGolden`.

---

## Pre-flight (required before Task 1)

Per spec §8.3 hard gates and §13 open questions, resolve these before opening the Phase 1 branch:

- [ ] **PR-merge freeze announcement channel** decided. Spec §11 commits to a 48h freeze post-Phase-1-merge. Team needs a known channel.
- [ ] **Wire library version** picked. Use latest stable at branch creation: `go get github.com/google/wire@latest`. Pin in this plan when chosen.
- [ ] **Branch protection on master** confirmed: force-push forbidden. (During Phase 0, an auto-push from a local merge made it to origin/master; verify settings catch this in future.)
- [ ] **CODEOWNERS for `api/internal/httpapi/contract/golden/`** is real (currently `@todd-l-am` placeholder; team can expand).
- [ ] **Mockery v3** confirmed in CI runners. Plan uses v3 syntax (`.mockery.yaml`).
- [ ] **Frontend coordination owner** identified. No frontend changes during Phase 1 branch lifetime.

When all six are checked, create the branch:

```bash
git checkout master
git pull --ff-only origin master
git checkout -b refactor/phase1-hex-arch
```

The plan assumes you start with `make test-cover-slices` showing all four business slices ≥ 80% and `make test-contract` green byte-strict.

---

## Target file structure (spec §4.1, summarized)

After Phase 1 lands, the layout is:

```
api/
├── cmd/
│   ├── api/                       (main.go + wire.go + wire_gen.go + main_test.go)
│   └── seeder/                    (extracted from internal/httpapi/devseed.go)
├── migrations/                    (unchanged)
├── pkg/
│   └── signing/                   (HMAC URL signer; shared with worker/)
└── internal/
    ├── auth/{domain,ports,service,adapters/{http,oauth},providers.go}
    ├── user/{domain,ports,service,adapters/{http,postgres},providers.go}
    ├── artwork/{domain,ports,service,adapters/{http,postgres},providers.go}
    ├── image/{domain,ports,service,adapters/{http,postgres},providers.go}
    └── infrastructure/
        ├── config/                (env loading)
        ├── database/              (GORM connection + Transactor + migrate runner)
        ├── logger/                (zerolog setup)
        ├── server/                (chi composition + WriteError + validator + RouteRegistrar)
        ├── storage/               (Storage port + localfs.go + r2.go)
        ├── health/                (healthz handler)
        └── testing/               (testcontainers harness — moved from internal/dbtest)
```

The complete current→target file-move map is in spec §4.2 (Task 2 references it).

---

## Conventions for every task

- **Branch:** `refactor/phase1-hex-arch` (set in pre-flight). All 11 commits land on this branch.
- **Commit message format:** `[phase1-hex-arch] <type>: <subject>` per `~/.claude/rules/git-conventions.md`. Types: `refactor` for layout-only, `feat` for new packages, `chore` for deps/tooling.
- **Commit boundaries are mandatory** per spec §9.1:
  - **C1**: layout moves precede behavioral changes
  - **C2**: pgx → GORM is the second-to-last commit before cleanup (commit 8 of 11)
  - **C3**: every commit individually leaves `make test-contract` green
  - **C4**: no commit both moves files AND changes driver/library semantics
- **Verification after every commit:**
  ```bash
  cd api
  make test-contract   # 56 cells must pass byte-strict
  make test            # full suite must pass
  go vet ./...         # must be clean
  ```
  If any of these fail, do NOT advance to the next task — fix the regression first.
- **No Co-Authored-By trailer.** No Claude attribution footer. No emojis in commits.
- **Weekly rebase** against master is mandatory if the branch lives > 7 days (spec §9.3).

---

## Task 1 — `chore: deps` (commit 1 of 11)

**Files:**
- Modify: `api/go.mod`
- Modify: `api/go.sum` (auto-generated)

**Goal:** Add Phase 1's runtime + dev dependencies in a single go.mod-only commit. No code changes; verify the project still builds and tests still pass on the existing pgx/slog stack.

- [ ] **Step 1: From `api/`, add the four runtime deps.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go get gorm.io/gorm@latest
go get gorm.io/driver/postgres@latest
go get github.com/google/wire@latest
go get github.com/rs/zerolog@latest
go get github.com/go-playground/validator/v10@latest
go mod tidy
```

- [ ] **Step 2: Verify the existing module still builds and tests pass.**

```bash
go build ./...
make test
```

Expected: clean build; all existing tests pass. Adding deps without using them must not break anything.

- [ ] **Step 3: Verify the contract suite is green.**

```bash
make test-contract
```

Expected: 56 cells PASS.

- [ ] **Step 4: Commit.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web
git add api/go.mod api/go.sum
git commit -m "[phase1-hex-arch] chore: add gorm, wire, zerolog, validator deps"
```

The commit changes ONLY `go.mod` and `go.sum`. No source files touched.

---

## Task 2 — `refactor: layout-move` (commit 2 of 11)

**Goal:** Move every file per spec §4.2 into its target slice/adapter location. Bodies unchanged. Imports updated to follow file moves. The codebase still runs on pgx, slog, and manual DI — only paths change. C1 + C4 enforced: this commit is purely mechanical.

**Files:** Per spec §4.2's file-move table (29 source moves). Plus all importers' import paths.

- [ ] **Step 1: Make new directories.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
mkdir -p \
  cmd/seeder \
  pkg/signing \
  internal/auth/{domain,ports,service,adapters/{http,oauth}} \
  internal/user/{domain,ports,service,adapters/{http,postgres}} \
  internal/artwork/{domain,ports,service,adapters/{http,postgres}} \
  internal/image/{domain,ports,service,adapters/{http,postgres}} \
  internal/infrastructure/{config,database,logger,server,storage,health,testing}
```

- [ ] **Step 2: Move signing files into pkg/signing.**

```bash
git mv internal/auth/sign.go pkg/signing/hmac.go
git mv internal/auth/imgurl.go pkg/signing/imgurl.go
```

Update `package` line in both files: `package auth` → `package signing`. Update every import that references the old paths:

```bash
grep -rln "local/art-web/api/internal/auth\"" --include='*.go' . \
  | xargs sed -i '' 's|"local/art-web/api/internal/auth/sign"|"local/art-web/api/pkg/signing"|g'
```

(Verify the sed pattern with `git grep`. macOS sed needs `-i ''`.)

- [ ] **Step 3: Move artwork files.**

```bash
git mv internal/artwork/repo.go internal/artwork/adapters/postgres/repo.go
git mv internal/artwork/tags.go internal/artwork/adapters/postgres/tag_repo.go
git mv internal/artwork/visibility.go internal/artwork/service/visibility.go
```

Update `package` lines:
- `internal/artwork/adapters/postgres/repo.go`: `package artwork` → `package postgres`
- `internal/artwork/adapters/postgres/tag_repo.go`: `package artwork` → `package postgres`
- `internal/artwork/service/visibility.go`: `package artwork` → `package service`

Repeat the import-rewriting pattern for every consumer.

- [ ] **Step 4: Move auth files.**

```bash
git mv internal/auth/handlers.go internal/auth/adapters/http/handlers.go
git mv internal/auth/middleware.go internal/auth/adapters/http/middleware.go
git mv internal/auth/jwt.go internal/auth/service/jwt.go
git mv internal/auth/google.go internal/auth/adapters/oauth/google.go
git mv internal/auth/provider.go internal/auth/ports/oauth.go
git mv internal/auth/randreader.go internal/auth/service/randreader.go
git mv internal/auth/randreader_test.go internal/auth/service/randreader_test.go
```

Update package declarations to match the new directory:
- `adapters/http/*.go` → `package http` (Go allows reused stdlib names; resolve via aliased import in consumers)
- `adapters/oauth/google.go` → `package oauth`
- `service/*.go` → `package service`
- `ports/oauth.go` → `package ports`

> **Naming hazard:** `package http` collides with `net/http` for consumers. Use a named alias when importing: `import authhttp "local/art-web/api/internal/auth/adapters/http"`. Spec §6.1 already uses this pattern.

- [ ] **Step 5: Move user files.**

```bash
git mv internal/user/repo.go internal/user/adapters/postgres/repo.go
git mv internal/user/repo_test.go internal/user/adapters/postgres/repo_test.go
git mv internal/user/repo_internal_test.go internal/user/adapters/postgres/repo_internal_test.go
git mv internal/user/slugify_test.go internal/user/adapters/postgres/slugify_test.go
```

Update packages: `repo.go` and the three test files → `package postgres` (internal tests stay `package postgres`; external tests become `package postgres_test`). The `Slugify` function moves with `repo.go` so the slugify test stays adjacent.

- [ ] **Step 6: Move image files.**

```bash
git mv internal/image/handler.go internal/image/adapters/http/handler.go
git mv internal/image/handler_test.go internal/image/adapters/http/handler_test.go
git mv internal/image/service.go internal/image/service/upload.go
git mv internal/image/service_test.go internal/image/service/upload_test.go
git mv internal/image/idprovider.go internal/image/service/idprovider.go
git mv internal/image/decode.go internal/image/service/decode.go
git mv internal/image/decode_test.go internal/image/service/decode_test.go
git mv internal/image/manifest.go internal/image/domain/manifest.go
git mv internal/image/manifest_test.go internal/image/domain/manifest_test.go
git mv internal/image/repo.go internal/image/adapters/postgres/repo.go
git mv internal/image/repo_test.go internal/image/adapters/postgres/repo_test.go
git mv internal/image/repo_internal_test.go internal/image/adapters/postgres/repo_internal_test.go
```

Update packages accordingly.

- [ ] **Step 7: Move httpapi → infrastructure/server. Slice handlers stay put for now (revised — see note below).**

```bash
git mv internal/httpapi/router.go internal/infrastructure/server/router.go
git mv internal/httpapi/render.go internal/infrastructure/server/respond.go
git mv internal/httpapi/middleware.go internal/infrastructure/server/middleware.go
git mv internal/httpapi/artworks.go internal/infrastructure/server/artworks.go
git mv internal/httpapi/artworks_test.go internal/infrastructure/server/artworks_test.go
git mv internal/httpapi/users.go internal/infrastructure/server/users.go
git mv internal/httpapi/tags.go internal/infrastructure/server/tags.go
git mv internal/httpapi/devseed.go internal/infrastructure/server/devseed.go
```

> **Note (revised after Task 2 review):** The earlier draft of this step said
> to move `artworks.go`/`users.go`/`tags.go` into `<slice>/adapters/http/` and
> `devseed.go` into `cmd/seeder/main.go`. That instruction was wrong for a
> layout-only commit. The handlers in those files are factory functions
> taking `*server.Deps` directly (e.g., `func meHandler(d *Deps) http.Handler`),
> so a peer slice package (`internal/artwork/adapters/http/`) cannot import
> them without either (a) defining per-slice deps structs or (b) converting
> handlers to accept narrow interfaces. Both changes are architectural —
> they belong in **Task 4 (Wire ProviderSets)** and **Task 7 (ports)**.
> Forcing the move during Task 2 either breaks the build or smuggles
> architectural changes into a "layout-only" commit, violating C1 + C4.
>
> Same reasoning for `devseed.go → cmd/seeder/main.go`: that requires
> removing the HTTP shell, adding `func main()`, and wiring CLI flags —
> exactly what **Task 10 (`feat: seeder`)** is for. Task 2 leaves
> `devseed.go` at `infrastructure/server/devseed.go` as a `package server`
> handler; Task 10 properly extracts it.
>
> The spec §4.1 target layout is reached **progressively** across Tasks 4,
> 7, and 10 — not in commit 2.

The httpapi tests in `internal/httpapi/` (testutil_test.go, privacy_matrix_test.go, error_codes_test.go, devseed_test.go, devseed_internal_test.go, me_test.go) are integration tests over the composed router. They need to find a new home — for now, move them all into `internal/infrastructure/server/`:

```bash
git mv internal/httpapi/testutil_test.go internal/infrastructure/server/testutil_test.go
git mv internal/httpapi/privacy_matrix_test.go internal/infrastructure/server/privacy_matrix_test.go
git mv internal/httpapi/error_codes_test.go internal/infrastructure/server/error_codes_test.go
git mv internal/httpapi/devseed_test.go internal/infrastructure/server/devseed_test.go
git mv internal/httpapi/devseed_internal_test.go internal/infrastructure/server/devseed_internal_test.go
git mv internal/httpapi/me_test.go internal/infrastructure/server/me_test.go
```

After Task 9 (http-errors) the test files may move per-slice; for now keep them together so the routing composition stays testable end-to-end. Update the package on each: `package httpapi` / `package httpapi_test` → `package server` / `package server_test`.

- [ ] **Step 8: Move storage and contract.**

```bash
git mv internal/storage/storage.go internal/infrastructure/storage/ports.go
git mv internal/storage/localfs.go internal/infrastructure/storage/localfs.go
git mv internal/storage/localfs_test.go internal/infrastructure/storage/localfs_test.go
git mv internal/storage/r2.go internal/infrastructure/storage/r2.go
git mv internal/storage/r2_test.go internal/infrastructure/storage/r2_test.go
git mv internal/db/pool.go internal/infrastructure/database/connection.go
git mv internal/db/pool_test.go internal/infrastructure/database/connection_test.go
git mv internal/db/migrate.go internal/infrastructure/database/migrate.go
git mv internal/db/migrate_test.go internal/infrastructure/database/migrate_test.go
git mv internal/dbtest/postgres.go internal/infrastructure/testing/postgres.go
git mv internal/dbtest/minio.go internal/infrastructure/testing/minio.go
git mv internal/dbtest/minio_test.go internal/infrastructure/testing/minio_test.go
git mv internal/dbtest/normalize.go internal/infrastructure/testing/normalize.go
git mv internal/dbtest/normalize_test.go internal/infrastructure/testing/normalize_test.go
git mv internal/dbtest/golden.go internal/infrastructure/testing/golden.go
git mv internal/dbtest/golden_test.go internal/infrastructure/testing/golden_test.go
git mv internal/dbtest/bootapp.go internal/infrastructure/testing/bootapp.go
git mv internal/dbtest/bootapp_test.go internal/infrastructure/testing/bootapp_test.go
```

The `internal/httpapi/contract/` directory stays where it is (the tests inside reference goldens via relative paths). Update its imports to point at the new `internal/infrastructure/testing` location.

```bash
# Update package declarations in the moved storage/db/dbtest files
# (storage → storage, database → database, dbtest → testing)
```

- [ ] **Step 9: Move healthz handler.**

The healthz inline handler in `httpapi/router.go:40-43` is small enough to keep inside the router file when it moves to `internal/infrastructure/server/router.go`. As a follow-up cleanup, extract to `internal/infrastructure/health/handler.go`:

```go
// internal/infrastructure/health/handler.go
package health

import "net/http"

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
```

Update the router to import + use it. This change is small enough to ship in this commit since it's just relocating ~3 lines of behavior.

- [ ] **Step 10: Update every import path that referenced an old location.**

This is the most error-prone step. Use grep to find every import that points at a now-moved path:

```bash
git grep -l '"local/art-web/api/internal/auth"' --
git grep -l '"local/art-web/api/internal/artwork"' --
git grep -l '"local/art-web/api/internal/user"' --
git grep -l '"local/art-web/api/internal/image"' --
git grep -l '"local/art-web/api/internal/storage"' --
git grep -l '"local/art-web/api/internal/db"' --
git grep -l '"local/art-web/api/internal/dbtest"' --
git grep -l '"local/art-web/api/internal/httpapi"' --
```

Then for each consumer, rewrite the import. Concrete examples (the most common ones):

`cmd/api/main.go`:
```
- "local/art-web/api/internal/auth"
+ authhttp    "local/art-web/api/internal/auth/adapters/http"
+ authoauth   "local/art-web/api/internal/auth/adapters/oauth"
+ authservice "local/art-web/api/internal/auth/service"
+ "local/art-web/api/pkg/signing"
- "local/art-web/api/internal/storage"
+ "local/art-web/api/internal/infrastructure/storage"
- "local/art-web/api/internal/db"
+ "local/art-web/api/internal/infrastructure/database"
- "local/art-web/api/internal/httpapi"
+ "local/art-web/api/internal/infrastructure/server"
```

(Repeat for every importer. The compile errors after a `go build ./...` will guide you — fix one, recompile, repeat.)

- [ ] **Step 11: Update `Deps` struct field types in BootApp.**

`internal/infrastructure/testing/bootapp.go` (was `internal/dbtest/bootapp.go`) constructs a `httpapi.Deps`. Now it constructs a `server.Deps`. Same struct, different package. Update accordingly.

The `Deps` struct in `infrastructure/server/router.go` (was `httpapi/router.go`) now references types from new packages. Update field types like:
```
Users    *user.Repo                    →  *userpostgres.Repo
Artworks *artwork.Repo                 →  *artworkpostgres.Repo
...etc
```

- [ ] **Step 12: Run the full suite from the new layout.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go build ./...
make test
make test-contract
go vet ./...
```

Expected: clean build; full suite passes; contract suite green byte-strict; vet clean.

If anything fails: the failures will be import-path or package-name mismatches, NOT semantic. Fix the path/name issue and re-run. Do NOT change function bodies.

- [ ] **Step 13: Commit.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web
git add -A api/
git commit -m "[phase1-hex-arch] refactor: move files to feature-sliced hexagonal layout"
```

The diff is enormous (every file in `api/internal/` either moves or has an import path updated). Reviewers focus on three things: (a) every move is in the §4.2 table, (b) function bodies unchanged (`git show --stat` should show many `R100` for pure renames), (c) `make test-contract` is green.

---

## Task 3 — `feat: infra-base` (commit 3 of 11)

**Goal:** Add the four `internal/infrastructure/` packages that exist purely as new code: `config`, `database` (cleanup of the old db package), `logger` (zerolog scaffolding — actual swap is Task 5), `server` (cleanup of the moved httpapi pieces). Each package gets its `providers.go` skeleton ready for Task 4 (Wire).

> **Note (revised after Task 3 review):** Step 1's `Load()` template below shows
> `JWT_KEY`/`URL_SIGN_KEY` and a `getenv("COOKIE_SECURE", "true") == "true"`
> boolean parse. Both are wrong:
> - **Env var names** must match the existing stack (`cmd/api/config.go`):
>   `JWT_SIGNING_KEY` and `WORKER_SIGNING_KEY`. The plan-template typo would
>   silently fail required-field validation in any environment provisioned
>   with the existing names.
> - **Boolean parse** must use `strconv.ParseBool` (accepts `"1"`/`"true"`/`"TRUE"`/etc.)
>   with a fail-safe default rather than literal `== "true"`. The literal
>   compare silently flips to `false` for `COOKIE_SECURE=TRUE` or `=1`,
>   which would disable secure cookies in production with no diagnostic.
> - **`PORT` validation:** `atoi` returns 0 silently on bad input, so
>   `PORT=abc` would have the server bind to `:0` (random ephemeral). Add
>   `if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 { return error }`.
>
> The actual implementation (commit `eec7c8f` + follow-up review fix) uses
> the corrected names, `parseBoolEnv` helper with `strconv.ParseBool`, and
> port range validation. Future re-runs of this task should follow the
> shipped code, not the template below.

**Files:**
- Create: `api/internal/infrastructure/config/config.go`
- Create: `api/internal/infrastructure/config/providers.go`
- Modify: `api/internal/infrastructure/database/connection.go` (rename existing `New` to `NewConnection`, return cleanup func)
- Create: `api/internal/infrastructure/database/providers.go`
- Create: `api/internal/infrastructure/database/transactor.go`
- Create: `api/internal/infrastructure/logger/logger.go`
- Create: `api/internal/infrastructure/logger/providers.go`
- Create: `api/internal/infrastructure/server/registrar.go`
- Modify: `api/internal/infrastructure/server/router.go` (will accept `[]RouteRegistrar`, see step 4)

- [ ] **Step 1: Write `internal/infrastructure/config/config.go`.**

```go
// api/internal/infrastructure/config/config.go
package config

import (
	"errors"
	"os"
	"strconv"
)

// AppConfig is the root config. Each sub-config is provided to consumers via
// wire.FieldsOf so each slice receives only what it needs (spec §6.7).
type AppConfig struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Storage  StorageConfig
	Image    ImageConfig
	Logger   LoggerConfig
}

type ServerConfig struct {
	AppEnv        string
	Frontend      string
	AllowedOrigin string
	Port          int
}

type DatabaseConfig struct {
	URL string
}

type AuthConfig struct {
	JWTKey  string
	SignKey string
	Cookie  CookieConfig
}

type CookieConfig struct {
	Domain string
	Secure bool
}

type StorageConfig struct {
	Backend string // "localfs" or "r2"
	Root    string // localfs only
	Bucket  string // r2 only
}

type ImageConfig struct {
	MaxBytes int64
}

type LoggerConfig struct {
	Level string // debug, info, warn, error
}

// Load reads env vars and returns AppConfig. Errors on missing required keys.
func Load() (*AppConfig, error) {
	cfg := &AppConfig{
		Server: ServerConfig{
			AppEnv:        getenv("APP_ENV", "production"),
			Frontend:      getenv("FRONTEND_URL", "http://localhost:3000/"),
			AllowedOrigin: getenv("ALLOWED_ORIGIN", "http://localhost:3000"),
			Port:          atoi(getenv("PORT", "8787")),
		},
		Database: DatabaseConfig{URL: os.Getenv("DATABASE_URL")},
		Auth: AuthConfig{
			JWTKey:  os.Getenv("JWT_KEY"),
			SignKey: os.Getenv("URL_SIGN_KEY"),
			Cookie: CookieConfig{
				Domain: getenv("COOKIE_DOMAIN", ""),
				Secure: getenv("COOKIE_SECURE", "true") == "true",
			},
		},
		Storage: StorageConfig{
			Backend: getenv("STORAGE_BACKEND", "localfs"),
			Root:    getenv("STORAGE_ROOT", "/tmp/art-web"),
			Bucket:  os.Getenv("R2_BUCKET"),
		},
		Image:  ImageConfig{MaxBytes: int64(atoi(getenv("IMAGE_MAX_BYTES", "26214401")))},
		Logger: LoggerConfig{Level: getenv("LOG_LEVEL", "info")},
	}
	if cfg.Database.URL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.Auth.JWTKey == "" || cfg.Auth.SignKey == "" {
		return nil, errors.New("JWT_KEY and URL_SIGN_KEY are required")
	}
	return cfg, nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
```

- [ ] **Step 2: Write `internal/infrastructure/config/providers.go`.**

```go
// api/internal/infrastructure/config/providers.go
package config

import "github.com/google/wire"

// ProviderSet exposes AppConfig + each sub-config via FieldsOf so consumers
// receive only the slice they need.
var ProviderSet = wire.NewSet(
	Load,
	wire.FieldsOf(new(*AppConfig),
		"Server", "Database", "Auth", "Storage", "Image", "Logger",
	),
)
```

- [ ] **Step 3: Write `internal/infrastructure/database/transactor.go`.**

Per spec §7.6:

```go
// api/internal/infrastructure/database/transactor.go
package database

import (
	"context"

	"gorm.io/gorm"
)

type ctxKey struct{}

// Transactor wraps a *gorm.DB and exposes WithinTx. Repo methods call
// DB(ctx, fallback) to get the active handle.
type Transactor struct{ db *gorm.DB }

func NewTransactor(db *gorm.DB) *Transactor { return &Transactor{db: db} }

// WithinTx runs fn inside a single transaction. The tx-bound *gorm.DB is
// stashed in ctx; repos retrieve it via DB(ctx, r.db).
func (t *Transactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, ctxKey{}, tx))
	})
}

// DB returns the active gorm handle: the transactional one if a tx is in flight,
// otherwise the fallback (request-scoped or root *gorm.DB).
func DB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(ctxKey{}).(*gorm.DB); ok {
		return tx
	}
	return fallback
}
```

This file imports `gorm.io/gorm` even though Task 8 (the GORM swap) hasn't landed yet. That's allowed per spec §9.1: this file is NEW and not yet referenced from any code path; it just sits compiled in the binary. C2 says "every prior commit builds and passes tests on pgx" — that's about the prod path, not unused imports.

- [ ] **Step 4: Write the `database/providers.go` and adjust `connection.go`.**

`internal/infrastructure/database/connection.go` (current — was `db/pool.go`) currently exposes `New(ctx, dsn) (*pgxpool.Pool, error)`. Add a no-yet-used GORM-aware constructor that returns `(T, func(), error)` per spec §6.5:

```go
// Append to connection.go
package database

import (
	"context"
	"errors"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewGormDB opens a GORM connection. Unused until Task 8 swaps consumers.
// Returns (*gorm.DB, cleanup, error) per spec §6.5.
func NewGormDB(cfg DatabaseConfig) (*gorm.DB, func(), error) {
	if cfg.URL == "" {
		return nil, nil, errors.New("database url required")
	}
	db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{
		// Don't auto-migrate; migrations live in api/migrations/*.sql
		// per spec §10.2 (forbidden).
	})
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return db, cleanup, nil
}
```

Add `DatabaseConfig` import (or alias, since it's in the `config` package — cross-import is needed):

```go
import (
	"local/art-web/api/internal/infrastructure/config"
)

// Replace `cfg DatabaseConfig` in NewGormDB signature with `cfg config.DatabaseConfig`
```

Wait — `database` package importing `config` is acceptable per the dependency rules (`infrastructure` is the integration layer). But to keep `database` reusable, you can also alias `type DatabaseConfig = config.DatabaseConfig` at top of the file. Pick whichever the team prefers; the import-cross is fine.

Then write `providers.go`:

```go
// api/internal/infrastructure/database/providers.go
package database

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewGormDB,
	NewTransactor,
)
```

- [ ] **Step 5: Write `internal/infrastructure/logger/logger.go`.**

```go
// api/internal/infrastructure/logger/logger.go
package logger

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"local/art-web/api/internal/infrastructure/config"
)

// New returns a configured zerolog.Logger. Production callers use this; tests
// inject their own via the Wire test ProviderSet.
func New(cfg config.LoggerConfig) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	log.Logger = logger
	return logger
}
```

- [ ] **Step 6: Write `internal/infrastructure/logger/providers.go`.**

```go
// api/internal/infrastructure/logger/providers.go
package logger

import "github.com/google/wire"

var ProviderSet = wire.NewSet(New)
```

- [ ] **Step 7: Write `internal/infrastructure/server/registrar.go`.**

Per spec §6.3:

```go
// api/internal/infrastructure/server/registrar.go
package server

import "github.com/go-chi/chi/v5"

// RouteRegistrar lets each slice's *Router type contribute its routes to the
// composed chi router. Implemented by authhttp.Router, userhttp.Router,
// artworkhttp.Router, imagehttp.Router.
type RouteRegistrar interface {
	RegisterRoutes(r chi.Router)
}

// ProvideRouteRegistrars is referenced in cmd/api/wire.go to assemble the
// registrar slice for ComposeRouter. Adding a slice means adding one parameter
// + one slice element here, plus its ProviderSet to wire.Build.
//
// NOTE: this provider returns []RouteRegistrar even though it builds it inline.
// Wire requires the function to exist so the slice can be a Wire-bound value.
func ProvideRouteRegistrars(
// auth, user, artwork, image *Router types fill in here in Task 7+
// when their per-slice routers are introduced.
) []RouteRegistrar {
	return nil
}
```

(The empty parameter list is intentional — Task 7 fills it in when the slice routers exist.)

- [ ] **Step 8: Don't touch the moved router yet. Verify build + tests.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go build ./...
make test
make test-contract
```

Expected: clean. The new infrastructure packages compile but aren't yet on any prod path.

- [ ] **Step 9: Commit.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web
git add api/internal/infrastructure/
git commit -m "[phase1-hex-arch] feat: add infrastructure base packages (config, logger, transactor, registrar)"
```

---

## Task 4 — `feat: wire` (commit 4 of 11)

**Goal:** Introduce `cmd/api/wire.go` (build-tag-gated source) + `cmd/api/wire_gen.go` (generated). Add a Makefile target. The injector exists but isn't yet called from `main.go` — that swap happens in Task 5 (logger) and ports/services come together in Task 7.

**Files:**
- Create: `api/cmd/api/wire.go` (with `//go:build wireinject` tag)
- Create: `api/cmd/api/wire_gen.go` (generated, NOT hand-edited)
- Modify: `api/Makefile` (add `make wire` target)
- Modify: `api/cmd/api/main.go` (no-op; wire integration deferred to later tasks)

- [ ] **Step 1: Write `cmd/api/wire.go`.**

```go
//go:build wireinject
// +build wireinject

// api/cmd/api/wire.go
package main

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"local/art-web/api/internal/infrastructure/config"
	"local/art-web/api/internal/infrastructure/database"
	"local/art-web/api/internal/infrastructure/logger"
	// auth, user, artwork, image, storage, server ProviderSets fill in
	// from Tasks 6+ as their per-slice providers.go files land.
)

// App is the root composition. Cleanup function shuts down server, then DB.
type App struct {
	Server *http.Server
	DB     *gorm.DB
	Logger zerolog.Logger
}

// InitializeApp returns the assembled *App + cleanup func. Wire generates the
// body in wire_gen.go.
func InitializeApp(ctx context.Context) (*App, func(), error) {
	wire.Build(
		config.ProviderSet,
		logger.ProviderSet,
		database.ProviderSet,
		// later: storage.ProviderSet, auth.ProviderSet, user.ProviderSet,
		//        artwork.ProviderSet, image.ProviderSet, server.ProviderSet,
		wire.Struct(new(App), "*"),
	)
	return nil, nil, nil // wire fills these in
}
```

- [ ] **Step 2: Add `make wire` and `make verify` targets to `api/Makefile`.**

Append (do not duplicate existing targets):

```make
.PHONY: wire wire-verify mocks

# Regenerate cmd/api/wire_gen.go from wire.go.
wire:
	cd cmd/api && go run github.com/google/wire/cmd/wire@latest ./...

# Fail if wire_gen.go is stale relative to wire.go (CI gate per spec §6.6).
wire-verify:
	cd cmd/api && go run github.com/google/wire/cmd/wire@latest ./...
	git diff --exit-code cmd/api/wire_gen.go

# Regenerate ports/<slice>/mocks via mockery v3.
mocks:
	go run github.com/vektra/mockery/v3@latest --config .mockery.yaml
```

- [ ] **Step 3: Generate `wire_gen.go`.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
make wire
```

Expected: creates `cmd/api/wire_gen.go`. The file has a `//go:build !wireinject` tag — opposite of `wire.go`. Check it in.

- [ ] **Step 4: Verify build with both files.**

```bash
go build ./cmd/api/...
make test
make test-contract
```

Expected: clean. The generated `wire_gen.go` provides a real (but tiny — only config/logger/database wired so far) `InitializeApp` function. Production `main.go` still uses its existing manual wiring; the Wire output is unused.

- [ ] **Step 5: Commit.**

```bash
git add api/cmd/api/wire.go api/cmd/api/wire_gen.go api/Makefile
git commit -m "[phase1-hex-arch] feat: introduce Wire injector + wire_gen.go (config/logger/database only)"
```

---

## Task 5 — `refactor: logger` (commit 5 of 11)

**Goal:** Replace every `log/slog` call site with zerolog. Mechanical refactor: same log levels, same fields, same call sites. After this task, `grep -r 'log/slog' api/` returns zero matches.

**Files:** Every file currently importing `log/slog`. Use grep to find them:

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
git grep -ln '"log/slog"'
```

Common patterns to rewrite:

| slog | zerolog |
|---|---|
| `slog.Info("msg", "k", v)` | `log.Info().Interface("k", v).Msg("msg")` |
| `slog.Error("msg", "err", err)` | `log.Error().Err(err).Msg("msg")` |
| `slog.Default()` | `log.Logger` (from `github.com/rs/zerolog/log`) |
| `*slog.Logger` parameter | `zerolog.Logger` parameter |

- [ ] **Step 1: Find every slog import.**

```bash
git grep -ln '"log/slog"' -- 'api/**/*.go'
```

Capture the list — each file needs a per-file edit.

- [ ] **Step 2: For each file, swap the import.**

```
- "log/slog"
+ "github.com/rs/zerolog/log"
```

If the file uses `*slog.Logger` as a parameter type, also import `github.com/rs/zerolog` and change the parameter to `zerolog.Logger`.

- [ ] **Step 3: For each file, rewrite call sites.**

Examples (the `cmd/api/main.go` pattern is most common):

```go
// before
slog.Info("api up", "port", cfg.Port)

// after
log.Info().Int("port", cfg.Port).Msg("api up")
```

```go
// before
slog.Error("flip rollback", "err", err, "image_id", id)

// after
log.Error().Err(err).Str("image_id", id).Msg("flip rollback")
```

zerolog field methods are typed (`Int`, `Str`, `Err`, `Interface` for fallback). If the value's static type isn't obvious, use `Interface`.

- [ ] **Step 4: Wire `zerolog.Logger` through the Deps struct if present.**

`internal/infrastructure/server/router.go` currently has no logger field. After this task it should:

```go
type Deps struct {
	// ... existing fields ...
	Logger zerolog.Logger
}
```

`BootApp` (in `infrastructure/testing/bootapp.go`) already creates a zerolog default; thread it through.

- [ ] **Step 5: Verify no slog references remain.**

```bash
git grep -n '"log/slog"' -- 'api/**/*.go' && echo "STILL FOUND" || echo "OK: zero slog imports"
git grep -n 'slog\.' -- 'api/**/*.go' && echo "STILL FOUND" || echo "OK: zero slog calls"
```

Both should print "OK".

- [ ] **Step 6: Run tests + contract suite.**

```bash
go vet ./...
make test
make test-contract
```

Expected: all pass. Logging is a side effect; the contract suite shouldn't notice.

- [ ] **Step 7: Commit.**

```bash
git add -A api/
git commit -m "[phase1-hex-arch] refactor: replace log/slog with zerolog across all packages"
```

---

## Task 6 — `feat: domain` (commit 6 of 11)

**Goal:** Per spec §7.2 + §7.3.2, introduce the four `<slice>/domain/` packages with entities + sentinel errors. Domain packages import nothing internal.

**Files:**
- Create: `api/internal/auth/domain/identity.go`
- Create: `api/internal/auth/domain/errors.go`
- Create: `api/internal/user/domain/user.go`
- Create: `api/internal/user/domain/errors.go`
- Create: `api/internal/artwork/domain/artwork.go`
- Create: `api/internal/artwork/domain/tag.go`
- Create: `api/internal/artwork/domain/errors.go`
- Create: `api/internal/artwork/domain/visibility.go`
- Create: `api/internal/image/domain/image.go`
- Create: `api/internal/image/domain/errors.go`
- (Note: `manifest.go` already moved to `image/domain/` in Task 2.)

- [ ] **Step 1: Auth domain.**

```go
// api/internal/auth/domain/identity.go
package domain

// Identity is the OAuth-provider-supplied profile after Exchange.
// Mirrors auth.Profile from current code; renaming for domain semantics.
type Identity struct {
	Subject     string // provider's stable user ID
	Email       string
	DisplayName string
	AvatarURL   string
}

// Session is what the auth service returns after successful sign-in.
type Session struct {
	UserID string
	Token  string // signed JWT
}
```

```go
// api/internal/auth/domain/errors.go
package domain

import "errors"

// HTTP-observable
var (
	ErrUnknownProvider = errors.New("auth: unknown provider")
	ErrBadState        = errors.New("auth: state cookie mismatch")
	ErrExchangeFailed  = errors.New("auth: oauth exchange failed")
)
```

- [ ] **Step 2: User domain.**

```go
// api/internal/user/domain/user.go
package domain

// User is the canonical user entity. Mirrors current user.User.
type User struct {
	ID          string
	Slug        string
	DisplayName string
	Email       string
	AvatarURL   string
}
```

```go
// api/internal/user/domain/errors.go
package domain

import "errors"

var (
	// HTTP-observable: 404 not_found in WriteError (§7.4)
	ErrNotFound = errors.New("user: not found")
)
```

- [ ] **Step 3: Artwork domain (most complex; see spec §7.2 + §7.3.2).**

```go
// api/internal/artwork/domain/visibility.go
package domain

type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

func ValidVisibility(s string) bool {
	return s == "public" || s == "private"
}
```

```go
// api/internal/artwork/domain/tag.go
package domain

type Tag struct {
	ID   string
	Name string
}
```

```go
// api/internal/artwork/domain/artwork.go
package domain

import "time"

type Artwork struct {
	ID            string
	UserID        string
	Title         string
	Description   *string
	Visibility    Visibility
	CoverPosition int
	CreatedAt     time.Time
	PublishedAt   *time.Time
	Tags          []Tag
	Images        []Image // populated when repo preloads
}

// Image is a slim view; full Image lives in internal/image/domain.
// Duplicated here to avoid cross-slice dependency (spec §4.3 cross-slice rule).
type Image struct {
	ID          string
	Position    int
	StorageKey  string
	ContentType string
	Width       int
	Height      int
	Blurhash    string
}

// Publish flips Visibility to public. Returns ErrAlreadyPublished if already
// public, ErrNoImages if zero images attached. The service catches
// ErrAlreadyPublished and returns nil to preserve the current 204 byte
// (spec §7.3.2).
func (a *Artwork) Publish() error {
	if a.Visibility == VisibilityPublic {
		return ErrAlreadyPublished
	}
	if len(a.Images) == 0 {
		return ErrNoImages
	}
	a.Visibility = VisibilityPublic
	return nil
}
```

```go
// api/internal/artwork/domain/errors.go
package domain

import "errors"

var (
	// HTTP-observable
	ErrNotFound = errors.New("artwork: not found") // → 404 not_found
	ErrNoImages = errors.New("artwork: no images") // service-internal

	// Internal-only — services translate before returning. Per spec §7.2.1:
	//   ErrForbidden      → translates to ErrNotFound (404, not 403)
	//   ErrAlreadyPublished → translates to nil (204, not 409)
	ErrForbidden        = errors.New("artwork: caller does not own resource")
	ErrAlreadyPublished = errors.New("artwork: already published")

	// Validation errors (set by service or domain constructor)
	ErrBadVisibility    = errors.New("artwork: bad visibility")
	ErrBadCoverPosition = errors.New("artwork: bad cover position")
)
```

- [ ] **Step 4: Image domain.**

```go
// api/internal/image/domain/image.go
package domain

type Image struct {
	ID            string
	ArtworkID     string
	ClientImageID string
	StorageKey    string
	SourceSHA256  string
	ContentType   string
	Position      int
	Width         int
	Height        int
	ByteSize      int
	Blurhash      string
}
```

```go
// api/internal/image/domain/errors.go
package domain

import "errors"

var (
	// HTTP-observable
	ErrTooLarge            = errors.New("image: file exceeds limit")            // 422 too_large
	ErrFingerprintMismatch = errors.New("image: client_image_id reused")        // 409 fingerprint_mismatch (Shape B)
	ErrPositionTaken       = errors.New("image: position taken")                // 412 position_taken
	ErrContentTypeMismatch = errors.New("image: content type mismatch")         // 422 content_type_mismatch (Shape B)
)
```

> **Note:** `image/domain/manifest.go` already exists from Task 2 (moved from `internal/image/manifest.go`). The package declaration there is already `package domain`. No new manifest file needed in this task.

- [ ] **Step 5: Verify domain packages have no internal imports.**

```bash
git grep -l 'local/art-web/api/internal' -- 'api/internal/*/domain/*.go' && echo "VIOLATION" || echo "OK: domain imports nothing internal"
```

Expected: nothing matches (domain pure).

- [ ] **Step 6: Verify everything still builds and tests pass.**

```bash
go build ./...
make test
make test-contract
go vet ./...
```

Expected: clean. Domain packages exist but aren't yet referenced from any code path — they'll be wired in Task 7 (ports).

- [ ] **Step 7: Commit.**

```bash
git add api/internal/auth/domain/ api/internal/user/domain/ api/internal/artwork/domain/ api/internal/image/domain/
git commit -m "[phase1-hex-arch] feat: introduce per-slice domain packages with entities and sentinels"
```

---

## Task 7 — `feat: ports` + service-interface swap (commit 7 of 11)

**Goal:** Per spec §6 + §7, introduce `<slice>/ports/` interface packages and update services to accept those interfaces. The concrete repo + adapter constructors stay where Task 2 put them; only the SIGNATURES of services change. Plus per-slice `<slice>/Router` types implementing the `RouteRegistrar` interface from Task 3.

**Files:** ~16 new ports + service files; many service files updated to take interfaces. Per-slice router files in `adapters/http/`.

This is a large task. Sub-step it per slice.

- [ ] **Step 1: Write `internal/auth/ports/oauth.go` (already moved by Task 2).**

`auth/ports/oauth.go` (was `auth/provider.go`) currently is:

```go
type Provider interface {
    Name() string
    AuthURL(state string) string
    Exchange(ctx context.Context, code string) (*Profile, error)
}
```

Update its package to `ports` (already done in Task 2). Also add the `UserLookup` and `JWTIssuer` interfaces:

```go
// api/internal/auth/ports/oauth.go
package ports

import (
	"context"

	"local/art-web/api/internal/auth/domain"
)

type OAuthProvider interface {
	Name() string
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*domain.Identity, error)
}

// UserLookup lets the auth service consult the user slice without importing it
// directly. Implemented by user/adapters/postgres/repo.UpsertOAuth.
type UserLookup interface {
	UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatar string) (string, error)
}

// JWTIssuer lets the auth service issue + verify tokens via service/jwt.go.
// Implemented by auth/service/jwt.go (after this task wires it).
//
// (Plan revision: the original draft used `ttlSeconds int`; the implemented
// signature uses `ttl time.Duration` because that's the canonical Go type for
// time intervals and matches the existing concrete `auth/service.JWT` API.)
type JWTIssuer interface {
	Issue(userID string, ttl time.Duration) (string, error)
	Verify(token string) (string, error)
}
```

- [ ] **Step 2: User ports.**

```go
// api/internal/user/ports/repo.go
package ports

import (
	"context"

	"local/art-web/api/internal/user/domain"
)

type UserRepository interface {
	UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatar string) (string, error)
	Get(ctx context.Context, id string) (*domain.User, error)
	GetBySlug(ctx context.Context, slug string) (*domain.User, error)
}
```

- [ ] **Step 3: Artwork ports.**

```go
// api/internal/artwork/ports/repo.go
package ports

import (
	"context"

	"local/art-web/api/internal/artwork/domain"
)

type ArtworkRepository interface {
	Get(ctx context.Context, id string) (*domain.Artwork, error)
	Create(ctx context.Context, uid, title, desc, visibility string) (string, error)
	PatchTitle(ctx context.Context, id, title string) error
	PatchDescription(ctx context.Context, id, desc string) error
	SetCoverPosition(ctx context.Context, id string, pos int) error
	Delete(ctx context.Context, id string) error
	PublicFeed(ctx context.Context, cursor FeedCursor, limit int) (*FeedPage, error)
	ListByUser(ctx context.Context, userID string, includePrivate bool, cursor FeedCursor, limit int) (*FeedPage, error)
	ListByTag(ctx context.Context, tag string, cursor FeedCursor, limit int) (*FeedPage, error)

	// Visibility-flip support (spec §7.6.1).
	PendingFlipMoves(ctx context.Context, artworkID, fromVis, toVis string) ([]FlipMove, error)
	UpdateImageStorageKeys(ctx context.Context, completed []FlipMove) error
	SetVisibility(ctx context.Context, id, visibility string) error
}

// Re-exported types live alongside the interface for clarity; the actual
// fields mirror what the current artwork.Repo returns.

type FeedCursor struct {
	Stamp time.Time
	ID    string
}

type FeedPage struct {
	Items      []domain.Artwork
	NextCursor *FeedCursor
}

type FlipMove struct {
	ID  string // image ID
	Src string // current storage key
	Dst string // target storage key
}
```

```go
// api/internal/artwork/ports/tag_repo.go
package ports

import (
	"context"

	"local/art-web/api/internal/artwork/domain"
)

type TagRepository interface {
	GetTags(ctx context.Context, artworkID string) ([]string, error)
	SetTags(ctx context.Context, artworkID string, tags []string) error
	ListByTag(ctx context.Context, name string, cursor FeedCursor, limit int) (*FeedPage, error)
}

var _ = domain.Tag{} // keep import warm if currently unused
```

```go
// api/internal/artwork/ports/rollback.go
package ports

import "context"

// RollbackReporter is invoked when a compensating storage-rollback move fails
// after a primary visibility-flip operation has already failed (a leak).
// Per spec §7.7, fired only when the *undo* itself fails.
type RollbackReporter interface {
	Report(ctx context.Context, imageID string, err error)
}
```

- [ ] **Step 4: Image ports.**

```go
// api/internal/image/ports/repo.go
package ports

import (
	"context"

	"local/art-web/api/internal/image/domain"
)

type ImageRepository interface {
	Insert(ctx context.Context, in InsertInput) (*InsertResult, error)
	FindByClientImageID(ctx context.Context, artworkID, clientImageID string) (*domain.Image, error)
	ListByArtwork(ctx context.Context, artworkID string) ([]domain.Image, error)
}

type InsertInput struct {
	ID, ArtworkID, ClientImageID, ContentType, StorageKey, Blurhash, SourceSHA256 string
	Position, Width, Height, ByteSize                                             int
}

type InsertResult struct {
	ID      string
	Existed bool
}
```

```go
// api/internal/image/ports/url_signer.go
package ports

import "time"

// URLSigner produces signed URLs for private storage. Implemented by
// pkg/signing.URLBuilder. Image service calls it for response URLs.
type URLSigner interface {
	Public(storageKey string) string
	Private(storageKey string, ttl time.Duration) string
}
```

- [ ] **Step 5: Update services to accept interfaces.**

Each `<slice>/service/*.go` file currently constructs concrete repos. Change service struct fields to `ports.X` types and constructor signatures to take interfaces.

Example for `internal/artwork/service/visibility.go` (currently uses concrete `*artwork.Repo`):

```go
// before (post-Task 2):
type VisibilityService struct {
    repo  *artworkpostgres.Repo
    store storage.Storage
}

// after:
type VisibilityService struct {
    repo     ports.ArtworkRepository
    storage  storage.Storage // already a port
    tx       *database.Transactor
    rollback ports.RollbackReporter
}

func NewVisibilityService(
    repo ports.ArtworkRepository,
    storage storage.Storage,
    tx *database.Transactor,
    rollback ports.RollbackReporter,
) *VisibilityService {
    return &VisibilityService{repo: repo, storage: storage, tx: tx, rollback: rollback}
}
```

Replace `RollbackLog` global (in old `visibility.go`) with the injected `rollback ports.RollbackReporter`. The default implementation lives in `artwork/adapters/log/rollback.go`:

```go
// api/internal/artwork/adapters/log/rollback.go
package log

import (
	"context"

	"github.com/rs/zerolog"
)

type ZerologReporter struct{ log zerolog.Logger }

func NewZerologReporter(log zerolog.Logger) *ZerologReporter {
	return &ZerologReporter{log: log}
}

func (r *ZerologReporter) Report(ctx context.Context, imageID string, err error) {
	r.log.Error().Err(err).Str("image_id", imageID).Msg("flip rollback")
}
```

- [ ] **Step 6: Add `<slice>/Router` types implementing `RouteRegistrar`.**

For each slice's `adapters/http/`, add a `router.go` (or extend the existing handlers file) with:

```go
// api/internal/artwork/adapters/http/router.go
package http

import (
	"github.com/go-chi/chi/v5"

	authhttp "local/art-web/api/internal/auth/adapters/http"
)

type Router struct {
	handler *Handler
	auth    *authhttp.Middleware
}

func NewRouter(h *Handler, auth *authhttp.Middleware) *Router {
	return &Router{handler: h, auth: auth}
}

func (router *Router) RegisterRoutes(r chi.Router) {
	r.Route("/artworks", func(r chi.Router) {
		r.Get("/", router.handler.ListFeed)
		r.Get("/{id}", router.handler.GetArtwork)
		r.With(router.auth.RequireUser()).Post("/", router.handler.Create)
		r.With(router.auth.RequireUser()).Patch("/{id}", router.handler.Patch)
		r.With(router.auth.RequireUser()).Delete("/{id}", router.handler.Delete)
		r.With(router.auth.RequireUser()).Post("/{id}/images", router.handler.UploadImages)
	})
	r.Get("/tags/{name}", router.handler.GetTag)
}
```

Repeat for each slice with the routes that slice owns.

- [ ] **Step 7: Update `infrastructure/server/registrar.go` to take the routers.**

```go
// api/internal/infrastructure/server/registrar.go (replace Task 3's stub)
package server

import (
	"github.com/go-chi/chi/v5"

	authhttp    "local/art-web/api/internal/auth/adapters/http"
	userhttp    "local/art-web/api/internal/user/adapters/http"
	artworkhttp "local/art-web/api/internal/artwork/adapters/http"
	imagehttp   "local/art-web/api/internal/image/adapters/http"
)

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

- [ ] **Step 8: Per-slice `providers.go` file.**

```go
// api/internal/artwork/providers.go
package artwork

import (
	"github.com/google/wire"

	httpadapter "local/art-web/api/internal/artwork/adapters/http"
	logadapter  "local/art-web/api/internal/artwork/adapters/log"
	"local/art-web/api/internal/artwork/adapters/postgres"
	"local/art-web/api/internal/artwork/ports"
	"local/art-web/api/internal/artwork/service"
)

var ProviderSet = wire.NewSet(
	postgres.NewArtworkRepo,
	postgres.NewTagRepo,
	logadapter.NewZerologReporter,
	service.NewArtworkService,
	service.NewVisibilityService,
	httpadapter.NewHandler,
	httpadapter.NewRouter,
	wire.Bind(new(ports.ArtworkRepository), new(*postgres.ArtworkRepo)),
	wire.Bind(new(ports.TagRepository),     new(*postgres.TagRepo)),
	wire.Bind(new(ports.RollbackReporter),  new(*logadapter.ZerologReporter)),
)
```

Repeat for `auth/providers.go`, `user/providers.go`, `image/providers.go`.

- [ ] **Step 9: Add slice ProviderSets to `cmd/api/wire.go`.**

```go
wire.Build(
    config.ProviderSet,
    logger.ProviderSet,
    database.ProviderSet,
    storage.ProviderSet,
    auth.ProviderSet,
    user.ProviderSet,
    artwork.ProviderSet,
    image.ProviderSet,
    server.ProviderSet, // composes *http.Server from RouteRegistrars
    wire.Struct(new(App), "*"),
)
```

Run `make wire` to regenerate `wire_gen.go`.

- [ ] **Step 10: Verify `wire_gen.go` is up-to-date and tests pass.**

```bash
make wire-verify  # must produce no diff
make test
make test-contract
go vet ./...
```

Expected: all pass. The contract suite runs the same routes, just composed via Wire-generated code now.

- [ ] **Step 11: Verify banned-import invariant.**

Spec §4.3:

```bash
# domain/ packages must import nothing internal
git grep -ln '"local/art-web/api/internal' -- 'api/internal/*/domain/*.go' && \
  echo "VIOLATION: domain imports internal" || echo "OK"

# ports/ packages must not import gorm, chi, zerolog
for pkg in gorm.io/gorm github.com/go-chi/chi github.com/rs/zerolog; do
  git grep -l "$pkg" -- 'api/internal/*/ports/*.go' && \
    echo "VIOLATION: ports imports $pkg" || true
done

# service/ packages must not import gorm or chi or any adapter package
for pat in gorm.io/gorm github.com/go-chi/chi 'adapters/'; do
  git grep -l "$pat" -- 'api/internal/*/service/*.go' && \
    echo "VIOLATION: service imports $pat" || true
done
```

If any check prints VIOLATION, fix the offending import before continuing.

- [ ] **Step 12: Commit.**

```bash
git add -A api/
git commit -m "[phase1-hex-arch] feat: introduce per-slice ports and service-interface swap"
```

---

## Task 8 — `refactor: gorm` (commit 8 of 11) — THE BIG ONE

**Goal:** Per spec §9.1 C2, this is the second-to-last commit before cleanup. Replace pgx with GORM in every `<slice>/adapters/postgres/repo.go`. Add `gorm_models.go` and `mappers.go` (pure functions). Use the `database.DB(ctx, r.db)` pattern from `Transactor` (Task 3).

**Constraint:** every prior commit (1-7) builds + passes tests on pgx. This commit flips to GORM. After this commit, `git grep -l "jackc/pgx" -- 'api/**/*.go'` returns zero matches except dep-pulled stuff in `go.mod`.

**Files:** Per-slice rewrites:
- `api/internal/user/adapters/postgres/repo.go` (rewrite)
- `api/internal/user/adapters/postgres/gorm_models.go` (new)
- `api/internal/user/adapters/postgres/mappers.go` (new)
- `api/internal/artwork/adapters/postgres/repo.go` (rewrite)
- `api/internal/artwork/adapters/postgres/tag_repo.go` (rewrite)
- `api/internal/artwork/adapters/postgres/gorm_models.go` (new)
- `api/internal/artwork/adapters/postgres/mappers.go` (new)
- `api/internal/image/adapters/postgres/repo.go` (rewrite)
- `api/internal/image/adapters/postgres/gorm_models.go` (new)
- `api/internal/image/adapters/postgres/mappers.go` (new)

Plus `internal/infrastructure/database/connection.go` — strip pgx code, keep only GORM `NewGormDB`.

- [ ] **Step 1: User repo + models + mappers.**

```go
// api/internal/user/adapters/postgres/gorm_models.go
package postgres

import "time"

// userModel is the GORM-tagged shape. `column` tags must match the existing
// schema EXACTLY — see migrations/0001_init.up.sql. NEVER use db.AutoMigrate
// (spec §10.2).
type userModel struct {
	ID            string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OAuthProvider string    `gorm:"column:oauth_provider;type:text;not null"`
	OAuthSubject  string    `gorm:"column:oauth_subject;type:text;not null"`
	Email         string    `gorm:"column:email;type:text;not null"`
	DisplayName   string    `gorm:"column:display_name;type:text;not null"`
	Slug          string    `gorm:"column:slug;type:text;uniqueIndex;not null"`
	AvatarURL     *string   `gorm:"column:avatar_url;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (userModel) TableName() string { return "users" }
```

```go
// api/internal/user/adapters/postgres/mappers.go
package postgres

import "local/art-web/api/internal/user/domain"

func toDomainUser(m *userModel) *domain.User {
	avatar := ""
	if m.AvatarURL != nil {
		avatar = *m.AvatarURL
	}
	return &domain.User{
		ID:          m.ID,
		Slug:        m.Slug,
		DisplayName: m.DisplayName,
		Email:       m.Email,
		AvatarURL:   avatar,
	}
}
```

```go
// api/internal/user/adapters/postgres/repo.go (rewrite — replace pgx version)
package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"local/art-web/api/internal/infrastructure/database"
	"local/art-web/api/internal/user/domain"
)

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// Slugify mirrors the previous public function; keep it exported because
// test code references it.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "user"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Get(ctx context.Context, id string) (*domain.User, error) {
	var m userModel
	err := database.DB(ctx, r.db).WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("user.Get: %w", err)
	}
	return toDomainUser(&m), nil
}

func (r *Repo) GetBySlug(ctx context.Context, slug string) (*domain.User, error) {
	var m userModel
	if err := database.DB(ctx, r.db).WithContext(ctx).First(&m, "slug = ?", slug).Error; err != nil {
		// Per spec §7.2.1 + §3, GetBySlug does NOT translate to ErrNotFound;
		// the handler treats any error as 404.
		return nil, err
	}
	return toDomainUser(&m), nil
}

func (r *Repo) UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatar string) (string, error) {
	db := database.DB(ctx, r.db).WithContext(ctx)

	// Existing user with same (provider, subject)?
	var existing userModel
	err := db.Where("oauth_provider = ? AND oauth_subject = ?", provider, subject).First(&existing).Error
	if err == nil {
		// UPDATE the email/display/avatar.
		updates := map[string]any{
			"email":        email,
			"display_name": displayName,
		}
		if avatar != "" {
			updates["avatar_url"] = avatar
		} else {
			updates["avatar_url"] = nil
		}
		if err := db.Model(&existing).Updates(updates).Error; err != nil {
			return "", fmt.Errorf("user.Update: %w", err)
		}
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("user.Lookup: %w", err)
	}

	// New user. Try with slug retry loop (mirrors current 50-attempt logic).
	base := Slugify(displayName)
	slug := base
	var avatarPtr *string
	if avatar != "" {
		avatarPtr = &avatar
	}
	for i := 0; i < 50; i++ {
		m := userModel{
			OAuthProvider: provider,
			OAuthSubject:  subject,
			Email:         email,
			DisplayName:   displayName,
			Slug:          slug,
			AvatarURL:     avatarPtr,
		}
		err := db.Create(&m).Error
		if err == nil {
			return m.ID, nil
		}
		switch uniqueConstraint(err) {
		case "users_slug_key":
			slug = fmt.Sprintf("%s-%d", base, i+2)
			continue
		case "users_oauth_provider_oauth_subject_key":
			// Race: someone else inserted between our Lookup and Create.
			if err := db.Where("oauth_provider = ? AND oauth_subject = ?", provider, subject).
				First(&existing).Error; err == nil {
				return existing.ID, nil
			}
			return "", errors.New("oauth conflict but row not found on re-read")
		default:
			return "", fmt.Errorf("user.Create: %w", err)
		}
	}
	return "", errors.New("slug exhausted")
}

func uniqueConstraint(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	if pgErr.SQLState() != "23505" {
		return ""
	}
	return pgErr.ConstraintName
}
```

The `pgconn` import is still needed because GORM's postgres driver wraps pgconn errors.

- [ ] **Step 2: Artwork repo + tag_repo + models + mappers.**

The artwork repo is the largest. Pattern is the same:

`gorm_models.go` declares `artworkModel`, `artworkImageModel`, `tagModel`, `artworkTagModel` structs with `gorm:"column:..."` tags matching `migrations/0001_init.up.sql` exactly.

`mappers.go` provides pure `toDomainArtwork(*artworkModel) *domain.Artwork`, `toDomainTag`, `toDomainImage` functions with field-by-field copies. Mappers panic on unexpected nil associations they expected (programmer error per spec §7.8).

`repo.go` rewrites every existing query to GORM. The tricky one is `PendingFlipMoves` and `UpdateImageStorageKeys` (visibility flip). For brevity I show the structure; see spec §7.6.1 for the exact flow:

```go
// api/internal/artwork/adapters/postgres/repo.go
type ArtworkRepo struct{ db *gorm.DB }

func NewArtworkRepo(db *gorm.DB) *ArtworkRepo { return &ArtworkRepo{db: db} }

func (r *ArtworkRepo) Get(ctx context.Context, id string) (*domain.Artwork, error) {
	var m artworkModel
	err := database.DB(ctx, r.db).WithContext(ctx).
		Preload("Tags").Preload("Images").
		First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("artwork.Get: %w", err)
	}
	return toDomainArtwork(&m), nil
}

func (r *ArtworkRepo) PendingFlipMoves(ctx context.Context, artworkID, fromVis, toVis string) ([]ports.FlipMove, error) {
	var images []artworkImageModel
	if err := database.DB(ctx, r.db).WithContext(ctx).
		Where("artwork_id = ?", artworkID).Find(&images).Error; err != nil {
		return nil, err
	}
	moves := make([]ports.FlipMove, 0, len(images))
	for _, im := range images {
		if !strings.HasPrefix(im.StorageKey, fromVis+"/") {
			return nil, fmt.Errorf("storage_key %q does not match visibility %q", im.StorageKey, fromVis)
		}
		dst := toVis + strings.TrimPrefix(im.StorageKey, fromVis)
		moves = append(moves, ports.FlipMove{ID: im.ID, Src: im.StorageKey, Dst: dst})
	}
	return moves, nil
}

func (r *ArtworkRepo) UpdateImageStorageKeys(ctx context.Context, completed []ports.FlipMove) error {
	db := database.DB(ctx, r.db).WithContext(ctx)
	for _, m := range completed {
		if err := db.Model(&artworkImageModel{}).Where("id = ?", m.ID).
			Update("storage_key", m.Dst).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *ArtworkRepo) SetVisibility(ctx context.Context, id, visibility string) error {
	updates := map[string]any{"visibility": visibility}
	if visibility == "public" {
		updates["published_at"] = gorm.Expr("COALESCE(published_at, now())")
	}
	return database.DB(ctx, r.db).WithContext(ctx).
		Model(&artworkModel{}).Where("id = ?", id).Updates(updates).Error
}

// ... Create, PatchTitle, PatchDescription, SetCoverPosition, Delete,
//     PublicFeed, ListByUser, ListByTag follow the same pattern.
```

- [ ] **Step 3: Image repo + models + mappers.**

Same pattern as user/artwork. The two unique-constraint cases (`artwork_images_artwork_id_client_image_id_key` → ErrFingerprintMismatch on SHA mismatch; `artwork_images_artwork_id_position_key` → ErrPositionTaken) translate via the same `uniqueConstraint(err)` helper.

- [ ] **Step 4: Update the visibility service to call into the new repo methods.**

`internal/artwork/service/visibility.go` was moved in Task 2 with body unchanged. Now it calls `r.repo.PendingFlipMoves(...)`, `r.repo.UpdateImageStorageKeys(...)`, `r.tx.WithinTx(...)`, etc. — instead of the old direct `pool.Query` / `pool.Begin`.

The "storage-first, DB-within-tx, compensate-on-failure" semantics from spec §7.6.1 must be preserved exactly. The pseudo-code in that spec section is the canonical reference.

- [ ] **Step 5: Strip pgx from `internal/infrastructure/database/connection.go`.**

Delete the old `New(ctx, dsn) *pgxpool.Pool` function. Only `NewGormDB(cfg)` remains. The `*pgxpool.Pool` type is no longer used anywhere — verify:

```bash
git grep -l 'pgxpool' -- 'api/**/*.go'
```

Should return only test files that use the pool to TruncateAll (move those to use the gorm raw `db.Exec("TRUNCATE ...")` instead) or are completely deletable.

- [ ] **Step 6: Update `internal/infrastructure/testing/postgres.go` to use GORM.**

The `dbtest.StartPostgres` returns a DSN. The TruncateAll helper currently takes a `func(ctx, sql, ...any) error`. Update its signature or its usage so callers can pass `db.WithContext(ctx).Exec(sql).Error` instead of `pool.Exec(ctx, sql)`.

- [ ] **Step 7: Update `internal/infrastructure/testing/bootapp.go`.**

The booter currently constructs all the repos with `*pgxpool.Pool`. Switch to `*gorm.DB` from `database.NewGormDB(...)`. Pass it through to each repo constructor.

- [ ] **Step 8: Run wire to update injector wiring.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
make wire
```

- [ ] **Step 9: Run the full suite.**

```bash
make test
make test-contract
go vet ./...
```

Expected: all green. The contract suite is the canary — if anything diverges by a byte, it fails. Common GORM gotchas to watch:
- `nil` vs `*string` for nullable columns (spec §5.3.1).
- `time.Time.Format(time.RFC3339)` matches what `httpapi/render.go` does today.
- Map-key alphabetization matches what Go's `json.Marshal(map[string]any)` produces — DTO struct field declaration order MUST match the alphabetic sort.
- `gen_random_uuid()` defaults work because GORM doesn't set `id` on Insert (the model uses `default:gen_random_uuid()`).

- [ ] **Step 10: Schema-drift check.**

Per spec §10.2:

```bash
# Apply migrations to a clean DB
docker compose up -d postgres  # or whichever local-pg invocation
golang-migrate -path migrations -database "$DATABASE_URL" up

# Generate schema GORM would produce (write a small dump tool)
go run ./internal/infrastructure/database/cmd/dump_gorm_schema > /tmp/gorm.sql
pg_dump --schema-only "$DATABASE_URL" > /tmp/live.sql
diff /tmp/gorm.sql /tmp/live.sql
```

Any diff = bug. Either fix GORM tags to match migration types or write the dump tool to ignore false-positives (whitespace, comment headers).

- [ ] **Step 11: Commit.**

```bash
git add -A api/
git commit -m "[phase1-hex-arch] refactor: replace pgx with GORM across all repos and connection layer"
```

---

## Task 9 — `feat: http-errors` (commit 9 of 11)

**Goal:** Replace per-handler `renderJSON(w, 400, map[...])` with a single `WriteError(w, log, err)` boundary in `infrastructure/server/errors.go`. Build the switch from `error_codes_observed.md`. Add the validator-translator per spec §7.5. Update DTOs to use validator tags per spec §7.3.1.

**Files:**
- Create: `api/internal/infrastructure/server/errors.go` (the WriteError function)
- Create: `api/internal/infrastructure/server/validation.go` (validator translator)
- Modify: every `<slice>/adapters/http/handlers.go` to call `WriteError(...)` instead of inline `renderJSON(...)` for error paths
- Create: `api/internal/<slice>/adapters/http/dto.go` (per-slice DTOs with validator tags)
- Add: `json_field_order` AST checker (CI gate per spec §10.1)

This task draws directly from `api/internal/httpapi/contract/error_codes_observed.md`. Do not invent codes; each switch arm must map to a captured pair.

- [ ] **Step 1: Write `internal/infrastructure/server/errors.go`.**

```go
// api/internal/infrastructure/server/errors.go
package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"

	artworkdomain "local/art-web/api/internal/artwork/domain"
	imagedomain   "local/art-web/api/internal/image/domain"
	userdomain    "local/art-web/api/internal/user/domain"
)

// HTTPError matches the current API's two distinct shapes (see spec §3):
//   { "error": "<code>" }                          — most paths
//   { "error": "<code>", "message": "<detail>" }   — parse/cursor errors
type HTTPError struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// http-layer parse-error sentinels — defined here so handlers can `errors.Is`
// against them without exposing parse internals.
var (
	ErrBadJSON   = errors.New("bad json")
	ErrBadCursor = errors.New("bad cursor")
)

// WriteError maps any error to its captured (status, code) pair from
// error_codes_observed.md. Domain.ErrForbidden and domain.ErrAlreadyPublished
// are NOT in this switch — services translate them per spec §7.2.1.
func WriteError(w http.ResponseWriter, log zerolog.Logger, err error) {
	switch {
	// --- 404 ---
	case errors.Is(err, userdomain.ErrNotFound),
		errors.Is(err, artworkdomain.ErrNotFound):
		write(w, http.StatusNotFound, HTTPError{Error: "not_found"})

	// --- 400 (parse) ---
	case errors.Is(err, ErrBadJSON):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_json"})
	case errors.Is(err, ErrBadCursor):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_cursor", Message: err.Error()})

	// --- 400 (validation) ---
	case errors.As(err, &validator.ValidationErrors{}):
		var verrs validator.ValidationErrors
		_ = errors.As(err, &verrs)
		write(w, http.StatusBadRequest, validationToHTTPError(verrs))

	// --- 412/422 (image) ---
	case errors.Is(err, imagedomain.ErrPositionTaken):
		write(w, http.StatusPreconditionFailed, HTTPError{Error: "position_taken"})
	case errors.Is(err, imagedomain.ErrFingerprintMismatch):
		write(w, http.StatusConflict, HTTPError{
			Error:   "fingerprint_mismatch",
			Message: "client_image_id reused with different bytes",
		})
	case errors.Is(err, imagedomain.ErrTooLarge):
		write(w, http.StatusUnprocessableEntity, HTTPError{Error: "too_large"})
	case errors.Is(err, imagedomain.ErrContentTypeMismatch):
		write(w, http.StatusUnprocessableEntity, HTTPError{
			Error:   "content_type_mismatch",
			Message: "body does not match declared content_type",
		})

	// --- 500 fallback (operation-name leaks per spec §3) ---
	default:
		// Use op-context wrapping to pick the right *_failed code.
		// E.g., service.go may wrap with fmt.Errorf("create: %w", err);
		// this switch reads the "create:" prefix and emits "create_failed".
		// See errorOpCode for the mapping built from contract goldens.
		log.Error().Err(err).Msg("internal error")
		write(w, http.StatusInternalServerError, HTTPError{Error: errorOpCode(err)})
	}
}

func write(w http.ResponseWriter, status int, body HTTPError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// errorOpCode reads the wrapped-error prefix to choose between the operation-
// specific 500 codes (create_failed, patch_failed, ...). Spec §3 documents
// these as "operation-name leaks" preserved into the contract.
func errorOpCode(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "create:"):
		return "create_failed"
	case strings.Contains(msg, "patch:"):
		return "patch_failed"
	case strings.Contains(msg, "flip:"):
		return "flip_failed"
	case strings.Contains(msg, "tag:"):
		return "tag_failed"
	case strings.Contains(msg, "delete:"):
		return "delete_failed"
	case strings.Contains(msg, "list:"):
		return "list_failed"
	case strings.Contains(msg, "user_lookup:"):
		return "user_lookup_failed"
	case strings.Contains(msg, "user:"):
		return "user_failed"
	case strings.Contains(msg, "upsert:"):
		return "upsert_failed"
	case strings.Contains(msg, "sign:"):
		return "sign_failed"
	case strings.Contains(msg, "upload:"):
		return "upload_failed"
	default:
		return "internal_error"
	}
}
```

(Add `"strings"` import.)

- [ ] **Step 2: Write `internal/infrastructure/server/validation.go`.**

```go
// api/internal/infrastructure/server/validation.go
package server

import "github.com/go-playground/validator/v10"

// validationToHTTPError maps a validator/v10 first-error to the matching
// captured code. Per spec §7.3.1, the current API only validates:
//   bad_visibility (oneof on Visibility field)
//   bad_cover_position (gte=0 on CoverPosition field)
// Everything else collapses to bad_json.
func validationToHTTPError(verrs validator.ValidationErrors) HTTPError {
	if len(verrs) == 0 {
		return HTTPError{Error: "bad_json"}
	}
	fe := verrs[0] // current API surfaces only the first failure
	switch {
	case fe.Field() == "Visibility" && fe.Tag() == "oneof":
		return HTTPError{Error: "bad_visibility"}
	case fe.Field() == "CoverPosition" && fe.Tag() == "gte":
		return HTTPError{Error: "bad_cover_position"}
	default:
		return HTTPError{Error: "bad_json"}
	}
}
```

- [ ] **Step 3: Per-slice DTOs.**

Per spec §7.3.1 — match current behavior NOT ideal behavior. For artwork (most common):

```go
// api/internal/artwork/adapters/http/dto.go
package http

// CreateArtworkRequest matches the current behavior:
//   - Visibility: oneof public/private (omitempty allows the empty-string default)
//   - Title, Description, Tags: NO validation (current API is permissive)
//
// Field declaration order is alphabetical so encoding/json's map-based output
// shape (current code) is reproduced byte-for-byte. The CI gate
// json_field_order checks this.
type CreateArtworkRequest struct {
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Title       string   `json:"title"`
	Visibility  string   `json:"visibility" validate:"omitempty,oneof=public private"`
}

type PatchArtworkRequest struct {
	CoverPosition *int     `json:"cover_position,omitempty" validate:"omitempty,gte=0"`
	Description   *string  `json:"description,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Title         *string  `json:"title,omitempty"`
	Visibility    *string  `json:"visibility,omitempty" validate:"omitempty,oneof=public private"`
}
```

> **CRITICAL:** Field order MUST be alphabetical by JSON tag (Description, Tags, Title, Visibility). The CI json_field_order gate enforces this.

- [ ] **Step 4: Wire `validator.Validate` into Wire's ProviderSet.**

```go
// api/internal/infrastructure/server/providers.go
package server

import (
	"github.com/go-playground/validator/v10"
	"github.com/google/wire"
)

func NewValidator() *validator.Validate {
	return validator.New()
}

var ProviderSet = wire.NewSet(
	NewValidator,
	ProvideRouteRegistrars,
	NewServer, // existing
)
```

Then run `make wire` to regenerate.

- [ ] **Step 5: Update each handler to use `WriteError` + DTO + validator.**

Pattern (from artwork PATCH):

```go
// api/internal/artwork/adapters/http/handlers.go
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	var req PatchArtworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, h.log, server.ErrBadJSON)
		return
	}
	if err := h.validator.Struct(&req); err != nil {
		server.WriteError(w, h.log, err) // validator.ValidationErrors flows through
		return
	}
	uid, _ := authmw.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.Patch(r.Context(), uid, id, mapPatchRequest(&req)); err != nil {
		server.WriteError(w, h.log, err) // domain.ErrNotFound → 404; etc.
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Repeat for every error-emitting handler. The image upload handler is the most complex; its 412/415/422 paths must continue to emit the same bytes.

- [ ] **Step 6: Add `json_field_order` CI gate.**

Write a small AST checker:

```go
// api/internal/infrastructure/server/cmd/json_field_order/main.go
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	root := "./internal"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	violations := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, "/dto.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts, ok := sp.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				var tags []string
				for _, f := range st.Fields.List {
					if f.Tag == nil {
						continue
					}
					tag := strings.Trim(f.Tag.Value, "`")
					if i := strings.Index(tag, `json:"`); i >= 0 {
						v := tag[i+6:]
						if j := strings.IndexAny(v, `,"`); j >= 0 {
							tags = append(tags, v[:j])
						}
					}
				}
				if !sort.StringsAreSorted(tags) {
					println(path+":", ts.Name.Name, "fields not in alphabetical JSON-tag order:", strings.Join(tags, ", "))
					violations++
				}
			}
		}
		return nil
	})
	if violations > 0 {
		os.Exit(1)
	}
}
```

Add Makefile target:

```make
json-field-order:
	go run ./internal/infrastructure/server/cmd/json_field_order
```

And a CI step in `.github/workflows/contract.yml` (or wherever). Spec §10.1 lists this as a required CI gate.

- [ ] **Step 7: Update CI workflow with the new gates.**

`.github/workflows/contract.yml` already has the contract job. Add jobs for:
- `wire-verify` — `make wire-verify` (zero-diff check)
- `mockery-check` — `mockery --check`
- `banned-imports` — write a small AST checker similar to json_field_order but checking `domain/`, `ports/`, `service/` import lists
- `json-field-order` — the gate from Step 6
- `coverage-threshold` — `make test-cover-slices` exits 1 if any slice < 80%

Each job is small. They run in parallel via the GitHub Actions matrix.

- [ ] **Step 8: Run everything.**

```bash
make wire
make test
make test-contract
make wire-verify
make json-field-order
go vet ./...
```

- [ ] **Step 9: Commit.**

```bash
git add -A api/
git commit -m "[phase1-hex-arch] feat: single WriteError boundary + validator translator + per-slice DTOs"
```

---

## Task 10 — `feat: seeder` (commit 10 of 11)

**Goal:** Per spec §4.2, `internal/infrastructure/server/devseed.go` (the file Task 2 deferred — see Task 2 Step 7 note) becomes a standalone binary at `cmd/seeder/main.go`. The router drops `/dev/seed`. Tests that need seeding call the binary or its package-level functions directly.

**Files:**
- Modify: `api/cmd/seeder/main.go` — turn from HTTP handler to standalone main with the same logic.
- Modify: `internal/infrastructure/server/router.go` — remove the `/dev/seed` route registration.
- Modify: `internal/infrastructure/testing/bootapp.go` — provide a seeding entry point that the contract suite calls (it currently does `POST /dev/seed?suffix=fixed1234` against the in-process server).

- [ ] **Step 1: Rewrite `cmd/seeder/main.go`.**

Convert from an HTTP handler with method `(*DevSeed).handle(w, r)` to:

```go
// api/cmd/seeder/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"local/art-web/api/internal/infrastructure/config"
	"local/art-web/api/internal/infrastructure/database"
	// ... slice repos, services
)

func main() {
	suffix := flag.String("suffix", "", "deterministic slug suffix (default random hex)")
	many := flag.Int("many", 0, "extra bulk artworks to seed")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil { fail(err) }
	db, cleanup, err := database.NewGormDB(cfg.Database)
	if err != nil { fail(err) }
	defer cleanup()

	out, err := Run(ctx, db, *suffix, *many)
	if err != nil { fail(err) }
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
}

// Run is the package-level entry point. The contract suite imports it
// directly instead of calling /dev/seed via HTTP.
func Run(ctx context.Context, db *gorm.DB, suffix string, many int) (*Output, error) {
	if suffix == "" {
		suffix = randHex(4)
	}
	// ... existing seeding logic, returning the seedResponse equivalent
}

type Output struct {
	AliceCookie string `json:"aliceCookie"`
	BobCookie   string `json:"bobCookie"`
	AliceSlug   string `json:"aliceSlug"`
	BobSlug     string `json:"bobSlug"`
	PID         string `json:"pId"`
	QID         string `json:"qId"`
}
```

- [ ] **Step 2: Update the contract matrix to call `seeder.Run` instead of the HTTP endpoint.**

`internal/httpapi/contract/matrix_test.go` currently does:

```go
r, err := noRedirectClient.Post(srv.URL+"/dev/seed?suffix=fixed1234", "application/json", nil)
```

Replace with a direct package call so the test doesn't depend on the dropped route:

```go
import "local/art-web/api/cmd/seeder"

seedOut, err := seeder.Run(t.Context(), bootedDB, "fixed1234", 0)
```

This requires the BootApp to expose the `*gorm.DB` it constructed. Update `bootapp.go`:

```go
type Booted struct {
	Server *httptest.Server
	DB     *gorm.DB
}

func BootApp(t testing.TB, opts BootOpts) *Booted { ... }
```

Update every call site of `BootApp` to use the new return type.

- [ ] **Step 3: Drop `/dev/seed` from the router.**

In `internal/infrastructure/server/router.go`, delete the block that registers `/dev/seed` when AppEnv == "test". The matrix no longer needs it.

- [ ] **Step 4: Verify the contract suite still passes.**

```bash
make test-contract
```

Expected: 56 cells PASS. The internal seeding mechanism changed but the byte-strict goldens are unchanged.

> **Hazard:** if any cell of the matrix happens to depend on /dev/seed being mounted (e.g., a future test that probes the route's existence), update the matrix accordingly. The current 56-cell matrix doesn't have such a probe.

- [ ] **Step 5: Verify everything else.**

```bash
make test
go vet ./...
```

- [ ] **Step 6: Commit.**

```bash
git add -A api/
git commit -m "[phase1-hex-arch] feat: extract /dev/seed to cmd/seeder; drop the route"
```

---

## Task 11 — `chore: cleanup` (commit 11 of 11)

**Goal:** Remove dead code. Final `go vet`, `staticcheck`, and `golangci-lint`. Confirm all merge gates per spec §10.1.

**Files:** Whatever needs removal — typically:
- Empty or near-empty files left over from layout-move
- Helper functions only used by code that's been replaced
- Imports that become unused after Task 9's WriteError consolidation

**Known carry-over items from earlier tasks (deferred here):**

- **`artwork/ports.TagRepository` is a dead interface.** Defined in `artwork/ports/tag_repo.go` (Task 7) but no consumer routes through it; `artwork/adapters/http/Handler` still holds the concrete `*artworkpostgres.TagsRepo` and `artwork/providers.go` lacks the `wire.Bind(new(ports.TagRepository), new(*postgres.TagsRepo))` that Task 7's plan called for. The Task 7 follow-up tried to add both and triggered a 28-pp drop in artwork's `-coverpkg` profile — a quirky `go test` instrumentation interaction we couldn't root-cause in time. Re-attempt here with a fresh look; possible angles: (a) move TagsRepo construction earlier so test caching invalidates cleanly, (b) try `go test -count=1 -race` to see if it's a race-related coverage skew, (c) test on Go 1.26+ where coverage was rewritten. If still puzzling, delete the `TagRepository` interface entirely.
- **Two artwork-repo cleanups deferred from Task 8 review** for the same `-coverpkg` mystery (any edit to artwork production files dropped artwork from 82.7% to ~64%):
  - `artwork/adapters/postgres/repo.go:73-80` — `artwork.Get` does NOT translate `gorm.ErrRecordNotFound` → `artworkdomain.ErrNotFound`. Every other `Get` in the codebase does. HTTP handlers collapse all errors to 404 today, so no current breakage; but `errors.Is(err, artworkdomain.ErrNotFound)` in service code (e.g. retry/circuit-breaker layers) will silently miss. Add the 3-line translation block that mirrors `user.Get`.
  - `artwork/adapters/postgres/repo.go:226` — `dst := toVis + strings.TrimPrefix(im.StorageKey, fromVis)` is correct but visually fragile (the guard checks `fromVis+"/"` prefix; the strip uses `fromVis` without the slash, relying on TrimPrefix retaining the slash). Replace with `dst := toVis + im.StorageKey[len(fromVis):]` plus a brief comment explaining the slash retention.
- **Wire deferral:** `cmd/api/wire.go` only wires `config + logger + database`; the four slice ProviderSets exist (Task 7) but aren't yet in `wire.Build`. `cmd/api/main.go` does the manual wiring. Either complete the integration here (will require typed-string providers for `JWT_SIGNING_KEY`, `WORKER_SIGNING_KEY`, OAuth client config, frontend URL) or accept manual wiring as the production pattern and document why.
- **Mockery v3:** Spec mentions `<slice>/ports/mocks/` but no `.mockery.yaml` exists yet. `make mocks` target exists but fails. Add `.mockery.yaml`, run `make mocks` to populate the directories, OR delete the `make mocks` target if mockery is being de-scoped.
- **Pre-existing gofmt-dirty files** (Phase 0 inheritance + Task 2 import re-order): `infrastructure/server/{devseed_test.go, privacy_matrix_test.go}` and `infrastructure/testing/normalize.go` — run `gofmt -w` to clean.
- **Makefile `@latest` version-pinning:** `wire@latest`, `staticcheck@latest`, `mockery/v3@latest` should pin to versions matching `go.mod` (currently `wire v0.7.0`) so CI is reproducible.

- [ ] **Step 1: Find unused exports.**

```bash
cd /Users/todd.lam/WORK/_TestScripts/art-web/api
go run honnef.co/go/tools/cmd/staticcheck@latest -checks 'U1000' ./...
```

`U1000` finds unused functions/types/vars. Review each finding; delete the unreachable ones. Be careful: a function used only by tests is NOT dead — it's adapter/seam code.

- [ ] **Step 2: Find empty packages.**

```bash
find api/internal -type d -empty
```

Remove empty directories.

- [ ] **Step 3: Final lint pass.**

```bash
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...
```

Fix any lint findings.

- [ ] **Step 4: Verify merge gates.**

Per spec §10.1, run each gate:

```bash
make test-contract     # contract_suite_parity (must be byte-strict green)
make test-cover-slices # coverage_threshold (each slice ≥ 80%)
make wire-verify       # wire_diff (zero diff)
go run ./internal/infrastructure/server/cmd/banned_imports
go run ./internal/infrastructure/server/cmd/json_field_order
make mocks && git diff --exit-code .  # mockery-check
go test ./cmd/api/... -run TestMainBootsCleanly   # smoke_test
# schema_drift requires a real DB; run the diff manually before merging
```

Each gate must pass. If any fails, that's a blocker.

- [ ] **Step 5: Final tests.**

```bash
make test
go test -race ./...
```

- [ ] **Step 6: Commit.**

```bash
git add -A api/
git commit -m "[phase1-hex-arch] chore: remove dead code, final lint passes, verify merge gates"
```

---

## Merge gates checklist (spec §10.1)

Before opening the Phase 1 PR, every box must be checked:

- [ ] `contract_suite_parity` — `make test-contract` green byte-strict
- [ ] `coverage_threshold` — `make test-cover-slices` shows all four ≥ 80%
- [ ] `wire_diff` — `make wire-verify` produces no diff
- [ ] `banned_imports` — AST checker passes (domain has no internal imports; ports has no framework; service has no adapter imports)
- [ ] `schema_drift` — GORM-generated schema matches live `pg_dump --schema-only`
- [ ] `smoke_test` — `cmd/api/main_test.go::TestMainBootsCleanly` passes
- [ ] `existing_unit_tests` — every commit on the branch passes the existing suite
- [ ] `mockery_check` — `mockery --check` produces no diff against committed mocks
- [ ] `json_field_order` — DTO struct fields are alphabetical by JSON tag
- [ ] No `Co-Authored-By:` trailers on any commit
- [ ] No Claude attribution footers
- [ ] All 11 commits start with `[phase1-hex-arch]`

---

## Post-merge timeline (spec §11)

Per spec §11, the team commits to:

1. **T+0 to T+48h** — pre-written `revert/refactor-go-scaffolding` PR is viable; only Phase-1-fix-forward PRs allowed; branch protection enforces.
2. **T+48h to T+7d** — fix-forward primary; revert is backup but requires conflict cleanup; non-trivial PRs queued.
3. **After T+7d** — fix-forward only; freeze lifted.

Canary requirements:
- Phase 1 merged commit runs on staging for ≥ 48h before production.
- First production deploy lands on a Tuesday morning.
- Monitoring (Grafana / Sentry / equivalent) reviewed daily during canary.

---

## Out of scope (the YAGNI list, spec §12)

These are explicitly NOT part of Phase 1:

- Adding new endpoints
- Changing API request/response shapes (including error shapes)
- Cleaning up `*_failed` internal-name leaks in error codes
- Changing migrations beyond what GORM tags require
- Adding new feature slices (`comments`, `likes`, etc.)
- Performance optimization unless required by snapshot diffs
- Test-style refactors not required by the layout move
- Documentation rewrites beyond updating module-level comments
- Making `cmd/seeder` production-grade (it's a dev tool; same scope as today)

If any of these creep into your PR, reviewers should reject the PR until they're removed.
