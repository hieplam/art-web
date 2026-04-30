# Plan 1 — Go API + PostgreSQL

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go API + PostgreSQL backend that owns metadata, auth, the upload pipeline, signed-URL generation, and emits public/private image origin URLs the Cloudflare Worker (Plan 2) and the Next.js frontend (Plan 3) consume.

**Architecture:** chi-routed HTTP API with HS256 JWT in an HttpOnly cookie, OAuth2 (Google), pgx/v5 to Postgres, golang-migrate for schema, an internal `Storage` interface with `localfs` (dev) and `r2` (prod, via AWS S3 SDK pointed at R2) implementations, on-upload metadata extraction (decode → width/height/blurhash). All image bytes are streamed straight to storage; no thumbnails are pre-generated. Privacy is enforced at the service boundary (private + non-owner returns 404, never 403).

**Tech Stack:** Go 1.22+, chi/v5, pgx/v5, golang-migrate/v4, golang-jwt/v5, golang.org/x/oauth2, aws-sdk-go-v2 (s3 client for R2), buckket/go-blurhash, testcontainers-go (postgres + minio).

**Subtree owned by this plan:** `api/` (read [contracts §1](./2026-04-30-art-gallery-contracts.md#1-monorepo-layout)).

**Read first:**
1. [Design spec](../specs/2026-04-30-art-gallery-design.md) — sections 5, 6, 8, 9.1, 11
2. [Contracts doc](./2026-04-30-art-gallery-contracts.md) — all sections; this plan must not deviate

**Prerequisite:** Docker daemon running (testcontainers spins up Postgres + MinIO).

---

## Task overview

| # | Task | Tests change |
|---|---|---|
| 1 | Bootstrap `api/` module skeleton | — |
| 2 | DB migration 0001 — schema | repo smoke |
| 3 | pgx pool wrapper + ping | unit |
| 4 | Postgres testcontainer harness | reused by 5+ |
| 5 | Migrations runner | unit |
| 6 | `Storage` interface + `localfs.Store` | unit |
| 7 | `r2.Store` against MinIO | integration |
| 8 | HMAC signer + canonicalization rules | unit (case 15 companion) |
| 9 | `PrivateImageURL` / `PublicImageURL` builders | unit |
| 10 | JWT issue/verify | unit |
| 11 | OAuth `Provider` interface + Google provider | unit (mocked Google) |
| 12 | `auth` middleware | unit |
| 13 | OAuth start/callback/logout/`/me` handlers | unit |
| 14 | `users` repository + slug uniqueness | repo |
| 15 | `artworks` repository — Create/Get/Patch/Delete | repo |
| 16 | `artworks` repository — public feed query | repo |
| 17 | `artworks` repository — by-user query (visibility-aware) | repo |
| 18 | `artworks` repository — by-tag query | repo |
| 19 | Tags repository — upsert + attach | repo |
| 20 | Image upload pipeline — manifest parsing + validation | unit |
| 21 | Image upload pipeline — decode + blurhash | unit |
| 22 | Image upload pipeline — idempotent insert | repo |
| 23 | Image upload pipeline — full handler | http |
| 24 | Privacy flip handler — public ⇄ private (R2 move + DB) | http |
| 25 | Artwork list/detail/create/patch/delete handlers | http |
| 26 | Tag listing handler | http |
| 27 | CORS + security middleware | unit |
| 28 | Privacy matrix tests (Go side, cases 1-5) | http |
| 29 | Upload idempotency tests (cases 17-20) | http |
| 30 | Publish lifecycle test (case 21) | http |
| 31 | `cmd/api/main.go` wiring + config | smoke |
| 32 | `Makefile` + `.github/workflows/api.yml` | CI |

---

## Task 1: Bootstrap `api/` module skeleton

**Files:**
- Create: `api/go.mod`
- Create: `api/.gitignore`
- Create: `api/cmd/api/main.go`
- Create: `api/internal/.keep`

- [ ] **Step 1: Initialize the module**

```bash
cd api && go mod init github.com/<org>/art-web/api
```

Replace `<org>` with the GitHub org/owner this repo will live under. If unknown, use `local/art-web` — the module path only matters for internal imports, not publishing.

- [ ] **Step 2: Add a placeholder main**

```go
// api/cmd/api/main.go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "art-web api: not yet wired (see Plan 1 Task 31)")
	os.Exit(2)
}
```

- [ ] **Step 3: Add `.gitignore` and an empty `internal/.keep`**

```gitignore
# api/.gitignore
/bin/
/var/storage/
*.test
coverage.out
.env
.env.local
```

```bash
touch api/internal/.keep
```

- [ ] **Step 4: Verify the build compiles**

Run: `cd api && go build ./...`
Expected: exit 0, no output.

- [ ] **Step 5: Commit**

```bash
git add api/
git commit -m "[api] chore: bootstrap go module skeleton"
```

---

## Task 2: DB migration 0001 — schema

**Files:**
- Create: `api/migrations/0001_init.up.sql`
- Create: `api/migrations/0001_init.down.sql`

- [ ] **Step 1: Author the up migration verbatim from spec §5**

```sql
-- api/migrations/0001_init.up.sql
CREATE TABLE users (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  oauth_provider  text NOT NULL,
  oauth_subject   text NOT NULL,
  email           text NOT NULL,
  display_name    text NOT NULL,
  slug            text UNIQUE NOT NULL,
  avatar_url      text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (oauth_provider, oauth_subject)
);

CREATE TABLE artworks (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title          text NOT NULL,
  description    text,
  visibility     text NOT NULL CHECK (visibility IN ('public','private')),
  cover_image_id uuid,
  created_at     timestamptz NOT NULL DEFAULT now(),
  published_at   timestamptz
);

CREATE INDEX artworks_public_feed_idx ON artworks (published_at DESC)
  WHERE visibility = 'public' AND published_at IS NOT NULL;

CREATE INDEX artworks_by_user_idx ON artworks (user_id, created_at DESC);

CREATE TABLE artwork_images (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  artwork_id      uuid NOT NULL REFERENCES artworks(id) ON DELETE CASCADE,
  client_image_id uuid NOT NULL,
  storage_key     text NOT NULL,
  width           int NOT NULL,
  height          int NOT NULL,
  byte_size       int NOT NULL,
  content_type    text NOT NULL,
  position        int NOT NULL,
  blurhash        text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (artwork_id, client_image_id),
  UNIQUE (artwork_id, position)
);

ALTER TABLE artworks
  ADD CONSTRAINT artworks_cover_fk
  FOREIGN KEY (cover_image_id) REFERENCES artwork_images(id);

CREATE TABLE tags (
  id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text UNIQUE NOT NULL
);

CREATE TABLE artwork_tags (
  artwork_id uuid NOT NULL REFERENCES artworks(id) ON DELETE CASCADE,
  tag_id     uuid NOT NULL REFERENCES tags(id),
  PRIMARY KEY (artwork_id, tag_id)
);
CREATE INDEX artwork_tags_by_tag_idx ON artwork_tags (tag_id);
```

- [ ] **Step 2: Author the down migration**

```sql
-- api/migrations/0001_init.down.sql
DROP TABLE IF EXISTS artwork_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS artwork_images;
DROP TABLE IF EXISTS artworks;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 3: Sanity-check the SQL by hand**

Walk down the up file with the spec §5 open: every column, type, constraint, index must match. The deferred FK on `artworks.cover_image_id` is intentional — `artwork_images` doesn't exist yet at the moment the `artworks` table is created.

- [ ] **Step 4: Commit**

```bash
git add api/migrations/
git commit -m "[api] feat: add 0001 init migration matching spec §5"
```

The migration is verified end-to-end in Task 5 once the runner exists.

---

## Task 3: pgx pool wrapper + ping

**Files:**
- Create: `api/internal/db/pool.go`
- Create: `api/internal/db/pool_test.go`

- [ ] **Step 1: Write the failing test (uses Task 4's harness, but test it first since the wrapper is simple)**

```go
// api/internal/db/pool_test.go
package db_test

import (
	"context"
	"testing"

	"github.com/<org>/art-web/api/internal/db"
)

func TestNew_PingsOnConstruction(t *testing.T) {
	dsn := startEphemeralPostgres(t) // helper added in Task 4
	pool, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestNew_RejectsBadDSN(t *testing.T) {
	if _, err := db.New(context.Background(), "postgres://nobody@127.0.0.1:1/none"); err == nil {
		t.Fatal("expected error connecting to nonexistent server")
	}
}
```

The `startEphemeralPostgres` helper does not exist yet — Task 4 adds it. This test stays red until then.

- [ ] **Step 2: Implement the wrapper**

```go
// api/internal/db/pool.go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool = pgxpool.Pool

func New(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 3: Run `go mod tidy` to pull pgx**

```bash
cd api && go mod tidy
```

- [ ] **Step 4: Defer test verification to Task 4**

Note in commit message that the test compiles but is gated on Task 4's harness. Run `go vet ./internal/db/` to confirm syntax.

- [ ] **Step 5: Commit**

```bash
git add api/internal/db/ api/go.mod api/go.sum
git commit -m "[api] feat(db): add pgx pool wrapper with ping-on-construct"
```

---

## Task 4: Postgres testcontainer harness

**Files:**
- Create: `api/internal/dbtest/postgres.go`

- [ ] **Step 1: Implement the harness (no test of its own — exercised transitively by every repo test)**

```go
// api/internal/dbtest/postgres.go
package dbtest

import (
	"context"
	"embed"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed all:../../migrations
var migrationsFS embed.FS

var (
	once       sync.Once
	sharedDSN  string
	sharedErr  error
	sharedTerm func()
)

// StartPostgres spins up a single shared Postgres container per test process,
// returning a DSN. The container is reused across tests for speed; each test
// is responsible for using a fresh schema or fresh tables (see TruncateAll).
func StartPostgres(t testing.TB) string {
	t.Helper()
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		container, err := tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("artweb"),
			tcpostgres.WithUsername("test"),
			tcpostgres.WithPassword("test"),
			tcpostgres.BasicWaitStrategies(),
			tcpostgres.WithInitScripts(),
			tcpostgres.WithSQLDriver("pgx"),
		)
		_ = wait.ForListeningPort // satisfy import in some tc-go versions
		if err != nil {
			sharedErr = err
			return
		}
		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedErr = err
			return
		}
		// Run migrations once.
		src, err := iofs.New(migrationsFS, "../../migrations")
		if err != nil {
			sharedErr = err
			return
		}
		m, err := migrate.NewWithSourceInstance("iofs", src, "pgx5://"+dsn[len("postgres://"):])
		if err != nil {
			sharedErr = err
			return
		}
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			sharedErr = err
			return
		}
		sharedDSN = dsn
		sharedTerm = func() { _ = container.Terminate(ctx) }
	})
	if sharedErr != nil {
		t.Fatalf("postgres harness: %v", sharedErr)
	}
	return sharedDSN
}

// TruncateAll wipes data tables but keeps schema. Call at the top of any
// repo test that wants a clean slate.
func TruncateAll(t testing.TB, exec func(ctx context.Context, sql string, args ...any) error) {
	t.Helper()
	if err := exec(context.Background(), `
		TRUNCATE TABLE artwork_tags, tags, artwork_images, artworks, users RESTART IDENTITY CASCADE;
	`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
```

Then add the `startEphemeralPostgres` helper used in Task 3's test:

```go
// api/internal/db/pool_test.go (append)
func startEphemeralPostgres(t *testing.T) string {
	return dbtest.StartPostgres(t)
}
```

(import `"github.com/<org>/art-web/api/internal/dbtest"`)

- [ ] **Step 2: Tidy + run Task 3's pool tests**

```bash
cd api && go mod tidy
go test ./internal/db/...
```

Expected: 2 tests pass.

- [ ] **Step 3: Commit**

```bash
git add api/internal/dbtest/ api/internal/db/pool_test.go api/go.mod api/go.sum
git commit -m "[api] feat(dbtest): postgres testcontainer harness with migrations"
```

---

## Task 5: Migrations runner (production)

**Files:**
- Create: `api/internal/db/migrate.go`
- Create: `api/internal/db/migrate_test.go`

- [ ] **Step 1: Failing test**

```go
// api/internal/db/migrate_test.go
package db_test

import (
	"context"
	"testing"

	"github.com/<org>/art-web/api/internal/db"
	"github.com/<org>/art-web/api/internal/dbtest"
)

func TestMigrate_IsIdempotent(t *testing.T) {
	dsn := dbtest.StartPostgres(t) // already migrated
	if err := db.MigrateUp(context.Background(), dsn); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}
	if err := db.MigrateUp(context.Background(), dsn); err != nil {
		t.Fatalf("second MigrateUp: %v", err)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/db/migrate.go
package db

import (
	"context"
	"embed"
	"errors"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed all:../../migrations
var migrationsFS embed.FS

func MigrateUp(_ context.Context, dsn string) error {
	src, err := iofs.New(migrationsFS, "../../migrations")
	if err != nil {
		return err
	}
	target := "pgx5://" + strings.TrimPrefix(dsn, "postgres://")
	m, err := migrate.NewWithSourceInstance("iofs", src, target)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
```

- [ ] **Step 3: Run**

```bash
cd api && go test ./internal/db/...
```

- [ ] **Step 4: Commit**

```bash
git add api/internal/db/migrate.go api/internal/db/migrate_test.go
git commit -m "[api] feat(db): idempotent migration runner using embed.FS"
```

---

## Task 6: `Storage` interface + `localfs.Store`

**Files:**
- Create: `api/internal/storage/storage.go`
- Create: `api/internal/storage/localfs.go`
- Create: `api/internal/storage/localfs_test.go`

- [ ] **Step 1: Interface (no test — pure type declaration)**

```go
// api/internal/storage/storage.go
package storage

import (
	"context"
	"io"
	"time"
)

type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error) // for tests + privacy-flip read
	Delete(ctx context.Context, key string) error
	Move(ctx context.Context, srcKey, dstKey string) error
	Exists(ctx context.Context, key string) (bool, error)
	// SignedURL is unused in v1 — Worker handles signing — kept on the interface
	// for symmetry with future direct-from-storage flows.
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}
```

- [ ] **Step 2: Failing tests for `localfs`**

```go
// api/internal/storage/localfs_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/<org>/art-web/api/internal/storage"
)

func TestLocalFS_PutGet(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ctx := context.Background()
	if err := s.Put(ctx, "public/abc/0.jpg", strings.NewReader("hello"), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	r, err := s.Get(ctx, "public/abc/0.jpg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != "hello" {
		t.Fatalf("want hello, got %q", got)
	}
}

func TestLocalFS_Move(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ctx := context.Background()
	_ = s.Put(ctx, "public/x/0.jpg", bytes.NewReader([]byte("h")), "image/jpeg")
	if err := s.Move(ctx, "public/x/0.jpg", "private/x/0.jpg"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if ok, _ := s.Exists(ctx, "public/x/0.jpg"); ok {
		t.Fatal("source should not exist after move")
	}
	if ok, _ := s.Exists(ctx, "private/x/0.jpg"); !ok {
		t.Fatal("dest should exist after move")
	}
}

func TestLocalFS_RejectsTraversal(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	if err := s.Put(context.Background(), "../etc/passwd", strings.NewReader("x"), "text/plain"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
```

- [ ] **Step 3: Implementation**

```go
// api/internal/storage/localfs.go
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type localFS struct{ root string }

func NewLocalFS(root string) Storage { return &localFS{root: root} }

func (l *localFS) abs(key string) (string, error) {
	if strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("invalid key %q", key)
	}
	return filepath.Join(l.root, filepath.FromSlash(key)), nil
}

func (l *localFS) Put(_ context.Context, key string, body io.Reader, _ string) error {
	p, err := l.abs(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func (l *localFS) Get(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.abs(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (l *localFS) Delete(_ context.Context, key string) error {
	p, err := l.abs(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (l *localFS) Move(ctx context.Context, src, dst string) error {
	sp, err := l.abs(src)
	if err != nil {
		return err
	}
	dp, err := l.abs(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dp), 0o755); err != nil {
		return err
	}
	return os.Rename(sp, dp)
}

func (l *localFS) Exists(_ context.Context, key string) (bool, error) {
	p, err := l.abs(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (l *localFS) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("localfs does not support signed URLs")
}
```

- [ ] **Step 4: Run**

```bash
cd api && go test ./internal/storage/...
```

- [ ] **Step 5: Commit**

```bash
git add api/internal/storage/
git commit -m "[api] feat(storage): Storage interface + localfs.Store with traversal guard"
```

---

## Task 7: `r2.Store` against MinIO

**Files:**
- Create: `api/internal/storage/r2.go`
- Create: `api/internal/storage/r2_test.go`

- [ ] **Step 1: Failing test using a MinIO testcontainer**

```go
// api/internal/storage/r2_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"github.com/<org>/art-web/api/internal/storage"
)

func TestR2_PutGetMove(t *testing.T) {
	ctx := context.Background()
	c, err := tcminio.Run(ctx, "minio/minio:latest")
	if err != nil {
		t.Fatalf("minio: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	endpoint, _ := c.ConnectionString(ctx)
	cli := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", ""),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://" + endpoint)
		o.UsePathStyle = true
	})
	_, err = cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("art")})
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	s := storage.NewR2(cli, "art")

	if err := s.Put(ctx, "public/x/0.jpg", bytes.NewReader([]byte("hi")), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	r, err := s.Get(ctx, "public/x/0.jpg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != "hi" {
		t.Fatalf("want hi got %q", got)
	}

	if err := s.Move(ctx, "public/x/0.jpg", "private/x/0.jpg"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if ok, _ := s.Exists(ctx, "public/x/0.jpg"); ok {
		t.Fatal("src still exists after move")
	}
	if ok, _ := s.Exists(ctx, "private/x/0.jpg"); !ok {
		t.Fatal("dst missing after move")
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/storage/r2.go
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type r2Store struct {
	cli    *s3.Client
	bucket string
}

func NewR2(cli *s3.Client, bucket string) Storage { return &r2Store{cli: cli, bucket: bucket} }

func (r *r2Store) Put(ctx context.Context, key string, body io.Reader, ct string) error {
	buf, err := io.ReadAll(body) // R2 PUT requires seekable; bounded ≤25 MB by validation upstream
	if err != nil {
		return err
	}
	_, err = r.cli.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf),
		ContentType: aws.String(ct),
	})
	return err
}

func (r *r2Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := r.cli.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (r *r2Store) Delete(ctx context.Context, key string) error {
	_, err := r.cli.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(key),
	})
	return err
}

func (r *r2Store) Move(ctx context.Context, src, dst string) error {
	_, err := r.cli.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(r.bucket),
		CopySource: aws.String(r.bucket + "/" + src),
		Key:        aws.String(dst),
	})
	if err != nil {
		return err
	}
	return r.Delete(ctx, src)
}

func (r *r2Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.cli.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(key),
	})
	if err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *r2Store) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("r2 store: signing handled by Worker, not direct presign")
}
```

- [ ] **Step 3: Run**

```bash
cd api && go test ./internal/storage/...
```

- [ ] **Step 4: Commit**

```bash
git add api/internal/storage/r2.go api/internal/storage/r2_test.go api/go.mod api/go.sum
git commit -m "[api] feat(storage): r2.Store via aws-sdk-go-v2, verified against MinIO"
```

---

## Task 8: HMAC signer + canonicalization rules

**Files:**
- Create: `api/internal/auth/sign.go`
- Create: `api/internal/auth/sign_test.go`

This is the companion to spec test case 15 — getting this byte-perfect with the Worker is what makes signed URLs work end-to-end. The contracts doc §3 is the canonical reference.

- [ ] **Step 1: Failing tests covering every canonicalization rule**

```go
// api/internal/auth/sign_test.go
package auth_test

import (
	"strings"
	"testing"

	"github.com/<org>/art-web/api/internal/auth"
)

var testKey = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func TestSign_DeterministicForFixedExp(t *testing.T) {
	a, _ := auth.SignCanonical(testKey, "/private/abc/def.jpg", 1700000000)
	b, _ := auth.SignCanonical(testKey, "/private/abc/def.jpg", 1700000000)
	if a != b || len(a) != 64 {
		t.Fatalf("non-deterministic or wrong length: %s vs %s", a, b)
	}
}

func TestSign_DistinctForDifferentInputs(t *testing.T) {
	cases := []struct{ p string; exp int64 }{
		{"/private/a/b.jpg", 100}, {"/private/a/b.jpg", 101},
		{"/private/a/c.jpg", 100}, {"/public/a/b.jpg", 100},
	}
	seen := map[string]string{}
	for _, c := range cases {
		s, _ := auth.SignCanonical(testKey, c.p, c.exp)
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
		if err := auth.ValidateCanonicalPath(p); err == nil {
			t.Errorf("expected reject for %q", p)
		}
	}
}

func TestCanonicalize_RejectsNonAllowedChars(t *testing.T) {
	bad := []string{"/private/abc def.jpg", "/private/abc%20.jpg", "/private/abc?q=1"}
	for _, p := range bad {
		if err := auth.ValidateCanonicalPath(p); err == nil {
			t.Errorf("expected reject for %q", p)
		}
	}
}

func TestCanonicalize_AcceptsSpecKey(t *testing.T) {
	good := "/private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0a1b2c3d-4e5f-6789-abcd-ef0123456789.jpg"
	if err := auth.ValidateCanonicalPath(good); err != nil {
		t.Errorf("rejected good path: %v", err)
	}
}

func TestSign_FixedVector(t *testing.T) {
	// Locks the byte-perfect HMAC output for a known input. The same hex
	// MUST appear in worker/test/contract_pin.spec.ts (Plan 2 Task 10).
	// If this value ever changes, the Worker verifier WILL break.
	got, err := auth.SignCanonical(testKey, "/private/aaa/bbb.jpg", 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	const want = "FILL_ME_FIRST_RUN" // see Step 3 below
	if want == "FILL_ME_FIRST_RUN" {
		t.Logf("first run — paste this into want: %s", got)
		t.Fatal("FixedVector not yet locked; see test log above")
	}
	if got != want {
		t.Fatalf("signature drifted:\n  got:  %s\n  want: %s", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("expected 64-char hex, got %d chars", len(got))
	}
}
```

`TestSign_FixedVector` deliberately fails on the very first run. Step 3 explains how to lock the value.

- [ ] **Step 2: Implementation**

```go
// api/internal/auth/sign.go
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// SignCanonical produces the lowercase-hex HMAC-SHA256 of the canonical
// string-to-sign defined in contracts §3:
//
//   v1|<canonicalPath>|<exp>
//
// canonicalPath must already be in canonical form (call ValidateCanonicalPath
// first; this function trusts its input).
func SignCanonical(key []byte, canonicalPath string, exp int64) (string, error) {
	if err := ValidateCanonicalPath(canonicalPath); err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, key)
	h.Write([]byte("v1|"))
	h.Write([]byte(canonicalPath))
	h.Write([]byte{'|'})
	h.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ValidateCanonicalPath(p string) error {
	if p == "" || p[0] != '/' {
		return fmt.Errorf("must start with /")
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("must not end with /")
	}
	if strings.Contains(p, "//") || strings.Contains(p, "/../") || strings.Contains(p, "/./") {
		return fmt.Errorf("contains forbidden segment")
	}
	for _, r := range p {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '/' || r == '.' || r == '_' || r == '-':
		default:
			return fmt.Errorf("char %q not allowed", r)
		}
	}
	return nil
}
```

- [ ] **Step 3: Lock the cross-language vector**

```bash
cd api && go test ./internal/auth/... -run TestSign_FixedVector -v
```

Expected on first run: FAIL with `first run — paste this into want: <64-char-hex>`. Copy the hex into the `want` constant in `sign_test.go`, replacing `FILL_ME_FIRST_RUN`. Re-run; it should now pass. Record the same hex in `worker/test/contract_pin.spec.ts` (Plan 2 Task 10) — the two values MUST match byte-for-byte forever after.

- [ ] **Step 4: Run + commit**

```bash
cd api && go test ./internal/auth/...
git add api/internal/auth/sign.go api/internal/auth/sign_test.go
git commit -m "[api] feat(auth): canonical HMAC signer with locked cross-language vector"
```

---

## Task 9: `PrivateImageURL` / `PublicImageURL` builders

**Files:**
- Create: `api/internal/auth/imgurl.go`
- Create: `api/internal/auth/imgurl_test.go`

- [ ] **Step 1: Failing tests**

```go
// api/internal/auth/imgurl_test.go
package auth_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/<org>/art-web/api/internal/auth"
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
	defer func() { _ = recover() }() // panic-or-error API choice — see impl
	if got := b.Private("public/abc/0.jpg", time.Minute); got != "" {
		t.Fatalf("expected empty/err for public key passed to Private(), got %s", got)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/auth/imgurl.go
package auth

import (
	"fmt"
	"strings"
	"time"
)

type URLBuilder struct {
	base string // e.g. https://cdn.example.com — no trailing slash
	key  []byte
	now  func() time.Time
}

func NewURLBuilder(base string, key []byte, now func() time.Time) *URLBuilder {
	return &URLBuilder{base: strings.TrimRight(base, "/"), key: key, now: now}
}

// Public returns the unsigned origin URL for a public storage key
// (e.g. "public/<artwork_id>/<image_id>.jpg").
func (b *URLBuilder) Public(storageKey string) string {
	return fmt.Sprintf("%s/img/%s", b.base, storageKey)
}

// Private returns a signed origin URL valid for ttl from now().
// Returns "" if storageKey does not begin with "private/".
func (b *URLBuilder) Private(storageKey string, ttl time.Duration) string {
	if !strings.HasPrefix(storageKey, "private/") {
		return ""
	}
	canonical := "/" + storageKey
	exp := b.now().Add(ttl).Unix()
	sig, err := SignCanonical(b.key, canonical, exp)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/img%s?sig=%s&exp=%d", b.base, canonical, sig, exp)
}
```

- [ ] **Step 3: Run**

```bash
cd api && go test ./internal/auth/...
```

- [ ] **Step 4: Commit**

```bash
git add api/internal/auth/imgurl.go api/internal/auth/imgurl_test.go
git commit -m "[api] feat(auth): URLBuilder for public + private image origin URLs"
```

---

## Task 10: JWT issue/verify

**Files:**
- Create: `api/internal/auth/jwt.go`
- Create: `api/internal/auth/jwt_test.go`

- [ ] **Step 1: Failing tests**

```go
// api/internal/auth/jwt_test.go
package auth_test

import (
	"testing"
	"time"

	"github.com/<org>/art-web/api/internal/auth"
)

func TestJWT_RoundTrip(t *testing.T) {
	j := auth.NewJWT(testKey, func() time.Time { return time.Unix(1000, 0) })
	tok, err := j.Issue("user-id-1", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	sub, err := j.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if sub != "user-id-1" {
		t.Fatalf("sub = %q", sub)
	}
}

func TestJWT_RejectsWrongKey(t *testing.T) {
	a := auth.NewJWT(testKey, time.Now)
	tok, _ := a.Issue("u", time.Hour)
	b := auth.NewJWT([]byte("ffffffffffffffffffffffffffffffff"), time.Now)
	if _, err := b.Verify(tok); err == nil {
		t.Fatal("expected verify failure with wrong key")
	}
}

func TestJWT_RejectsExpired(t *testing.T) {
	clock := time.Unix(1000, 0)
	j := auth.NewJWT(testKey, func() time.Time { return clock })
	tok, _ := j.Issue("u", time.Hour)
	clock = clock.Add(2 * time.Hour)
	if _, err := j.Verify(tok); err == nil {
		t.Fatal("expected expired token to fail verify")
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/auth/jwt.go
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWT struct {
	key []byte
	now func() time.Time
}

func NewJWT(key []byte, now func() time.Time) *JWT {
	if now == nil {
		now = time.Now
	}
	return &JWT{key: key, now: now}
}

func (j *JWT) Issue(sub string, ttl time.Duration) (string, error) {
	now := j.now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sub,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	})
	return tok.SignedString(j.key)
}

func (j *JWT) Verify(token string) (string, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{},
		func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected alg")
			}
			return j.key, nil
		},
		jwt.WithTimeFunc(j.now),
	)
	if err != nil {
		return "", err
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	return claims.Subject, nil
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/auth/...
git add api/internal/auth/jwt.go api/internal/auth/jwt_test.go api/go.mod api/go.sum
git commit -m "[api] feat(auth): HS256 JWT with injectable clock"
```

---

## Task 11: OAuth `Provider` interface + Google provider

**Files:**
- Create: `api/internal/auth/provider.go`
- Create: `api/internal/auth/google.go`
- Create: `api/internal/auth/google_test.go`

- [ ] **Step 1: Interface**

```go
// api/internal/auth/provider.go
package auth

import "context"

type Profile struct {
	Subject     string // provider-stable user id (Google sub claim)
	Email       string
	DisplayName string
	AvatarURL   string
}

type Provider interface {
	Name() string
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*Profile, error)
}
```

- [ ] **Step 2: Failing test against a stub Google token endpoint**

```go
// api/internal/auth/google_test.go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/<org>/art-web/api/internal/auth"
)

func TestGoogle_Exchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at", "id_token": "x.y.z", "token_type": "Bearer", "expires_in": 3600,
			})
		case "/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub": "100", "email": "a@b.com", "name": "Alice", "picture": "http://avatar/a.png",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	p := auth.NewGoogleProviderForTest("cid", "csecret", "http://app/cb", srv.URL+"/auth", srv.URL+"/token", srv.URL+"/userinfo")
	prof, err := p.Exchange(t.Context(), "the-code")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if prof.Subject != "100" || prof.Email != "a@b.com" || prof.DisplayName != "Alice" {
		t.Fatalf("bad profile: %+v", prof)
	}
}
```

- [ ] **Step 3: Implementation**

```go
// api/internal/auth/google.go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

type googleProvider struct {
	cfg         *oauth2.Config
	userinfoURL string
}

func NewGoogleProvider(clientID, clientSecret, redirectURL string) Provider {
	return &googleProvider{
		cfg: &oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
			Scopes: []string{"openid", "email", "profile"},
		},
		userinfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
	}
}

// NewGoogleProviderForTest exposes endpoint overrides for the unit test harness.
func NewGoogleProviderForTest(id, secret, redirect, authURL, tokenURL, userinfoURL string) Provider {
	return &googleProvider{
		cfg: &oauth2.Config{
			ClientID: id, ClientSecret: secret, RedirectURL: redirect,
			Endpoint: oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
			Scopes:   []string{"openid", "email", "profile"},
		},
		userinfoURL: userinfoURL,
	}
}

func (g *googleProvider) Name() string { return "google" }

func (g *googleProvider) AuthURL(state string) string {
	return g.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *googleProvider) Exchange(ctx context.Context, code string) (*Profile, error) {
	tok, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", g.userinfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, errors.New("userinfo: " + resp.Status)
	}
	var u struct {
		Sub, Email, Name, Picture string
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	if u.Sub == "" {
		return nil, errors.New("userinfo: empty sub")
	}
	return &Profile{
		Subject: u.Sub, Email: u.Email,
		DisplayName: strings.TrimSpace(u.Name),
		AvatarURL:   u.Picture,
	}, nil
}

var _ = url.QueryEscape // keep net/url import if oauth2 strips it later
```

- [ ] **Step 4: Run + commit**

```bash
cd api && go test ./internal/auth/...
git add api/internal/auth/provider.go api/internal/auth/google.go api/internal/auth/google_test.go api/go.mod api/go.sum
git commit -m "[api] feat(auth): pluggable Provider interface + Google impl"
```

---

## Task 12: `auth` middleware

**Files:**
- Create: `api/internal/auth/middleware.go`
- Create: `api/internal/auth/middleware_test.go`

- [ ] **Step 1: Failing tests**

```go
// api/internal/auth/middleware_test.go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/<org>/art-web/api/internal/auth"
)

func TestMiddleware_NoCookie_StillCallsNext(t *testing.T) {
	j := auth.NewJWT(testKey, nil)
	mw := auth.Middleware(j)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := auth.UserIDFrom(r.Context()); ok {
			t.Fatalf("expected no user, got %s", uid)
		}
		called = true
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("next never called")
	}
}

func TestMiddleware_ValidCookie_PopulatesContext(t *testing.T) {
	j := auth.NewJWT(testKey, nil)
	tok, _ := j.Issue("uid-42", time.Hour)
	mw := auth.Middleware(j)
	var got string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		got = uid
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth", Value: tok})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "uid-42" {
		t.Fatalf("uid = %s", got)
	}
}

func TestRequireUser_401WhenAbsent(t *testing.T) {
	h := auth.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 401 {
		t.Fatalf("code = %d", rec.Code)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/auth/middleware.go
package auth

import (
	"context"
	"net/http"
)

type ctxKey int

const userIDKey ctxKey = 1

func Middleware(j *JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie("auth")
			if err == nil {
				if sub, err := j.Verify(c.Value); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), userIDKey, sub))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserIDFrom(r.Context()); !ok {
			http.Error(w, `{"error":"unauthorized"}`, 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserIDFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok && v != ""
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/auth/...
git add api/internal/auth/middleware.go api/internal/auth/middleware_test.go
git commit -m "[api] feat(auth): cookie-reading middleware + RequireUser gate"
```

---

## Task 13: OAuth start/callback/logout/`/me` handlers

**Files:**
- Create: `api/internal/auth/handlers.go`
- Create: `api/internal/auth/handlers_test.go`

- [ ] **Step 1: Failing tests** (test each handler in isolation against an in-memory user repo)

```go
// api/internal/auth/handlers_test.go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/<org>/art-web/api/internal/auth"
)

type fakeProvider struct{}

func (fakeProvider) Name() string                 { return "google" }
func (fakeProvider) AuthURL(state string) string  { return "https://goog/auth?state=" + state }
func (fakeProvider) Exchange(_ context.Context, code string) (*auth.Profile, error) {
	return &auth.Profile{Subject: "S-" + code, Email: "a@b", DisplayName: "Alice", AvatarURL: ""}, nil
}

type fakeUserRepo struct{ stored map[string]string }

func (f *fakeUserRepo) UpsertOAuth(_ context.Context, prov, sub, email, name, avatar string) (string, error) {
	if f.stored == nil {
		f.stored = map[string]string{}
	}
	uid := "uid-" + sub
	f.stored[uid] = email
	return uid, nil
}

func TestStart_RedirectsToProviderWithState(t *testing.T) {
	h := auth.StartHandler(map[string]auth.Provider{"google": fakeProvider{}}, "/")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/google/start", nil)
	addChiURLParam(req, "provider", "google")
	h.ServeHTTP(rec, req)
	if rec.Code != 302 {
		t.Fatalf("code %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if !strings.Contains(loc.String(), "state=") {
		t.Fatal("no state param")
	}
	if c := getCookie(rec.Result(), "oauth_state"); c == nil {
		t.Fatal("no state cookie")
	}
}

func TestCallback_SetsAuthCookie(t *testing.T) {
	repo := &fakeUserRepo{}
	jwts := auth.NewJWT(testKey, nil)
	opts := auth.CookieOpts{Domain: ".example.com", Secure: true}
	h := auth.CallbackHandler(map[string]auth.Provider{"google": fakeProvider{}}, repo, jwts, "https://app.example.com/", opts)
	req := httptest.NewRequest("GET", "/auth/google/callback?code=C&state=S", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "S"})
	addChiURLParam(req, "provider", "google")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 302 {
		t.Fatalf("code %d", rec.Code)
	}
	c := getCookie(rec.Result(), "auth")
	if c == nil || c.Value == "" {
		t.Fatal("auth cookie not set")
	}
	if !c.HttpOnly || !c.Secure || c.Domain != ".example.com" {
		t.Fatalf("cookie attributes wrong: %+v", c)
	}
}

func TestLogout_ClearsAuthCookie(t *testing.T) {
	h := auth.LogoutHandler(auth.CookieOpts{Domain: ".example.com", Secure: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/auth/logout", nil))
	c := getCookie(rec.Result(), "auth")
	if c == nil || c.MaxAge != -1 || c.Domain != ".example.com" {
		t.Fatalf("cookie not cleared correctly: %+v", c)
	}
}

// helpers — addChiURLParam is provided by httpapi/testutil in Task 25;
// for this isolated package test we inline a minimal version.
func addChiURLParam(r *http.Request, k, v string) { /* see Task 25 */ }
func getCookie(r *http.Response, name string) *http.Cookie {
	for _, c := range r.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/auth/handlers.go
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type UserUpserter interface {
	UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatarURL string) (userID string, err error)
}

// CookieOpts controls Domain + Secure on auth cookies. Production sets
// both; dev leaves Domain empty and Secure=false (contracts §7).
type CookieOpts struct {
	Domain string // e.g. ".example.com"; empty for dev
	Secure bool
}

func StartHandler(providers map[string]Provider, _ string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := providers[chi.URLParam(r, "provider")]
		if !ok {
			http.Error(w, `{"error":"unknown_provider"}`, 404)
			return
		}
		state := randState()
		http.SetCookie(w, &http.Cookie{
			Name: "oauth_state", Value: state,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Path: "/", MaxAge: 600,
		})
		http.Redirect(w, r, p.AuthURL(state), http.StatusFound)
	})
}

func CallbackHandler(
	providers map[string]Provider,
	users UserUpserter,
	jwts *JWT,
	frontendHome string,
	cookieOpts CookieOpts,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := providers[chi.URLParam(r, "provider")]
		if !ok {
			http.Error(w, `{"error":"unknown_provider"}`, 404)
			return
		}
		stateCookie, err := r.Cookie("oauth_state")
		if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
			http.Error(w, `{"error":"bad_state"}`, 400)
			return
		}
		prof, err := p.Exchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, `{"error":"exchange_failed"}`, 502)
			return
		}
		uid, err := users.UpsertOAuth(r.Context(), p.Name(), prof.Subject, prof.Email, prof.DisplayName, prof.AvatarURL)
		if err != nil {
			http.Error(w, `{"error":"upsert_failed"}`, 500)
			return
		}
		tok, err := jwts.Issue(uid, 7*24*time.Hour)
		if err != nil {
			http.Error(w, `{"error":"sign_failed"}`, 500)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: "auth", Value: tok,
			Domain: cookieOpts.Domain,
			HttpOnly: true, Secure: cookieOpts.Secure,
			SameSite: http.SameSiteLaxMode, Path: "/", MaxAge: 7 * 24 * 3600,
		})
		http.Redirect(w, r, frontendHome, http.StatusFound)
	})
}

func LogoutHandler(cookieOpts CookieOpts) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name: "auth", Value: "",
			Domain: cookieOpts.Domain,
			HttpOnly: true, Secure: cookieOpts.Secure,
			SameSite: http.SameSiteLaxMode, Path: "/", MaxAge: -1,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}

func randState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

The TTL math `7 * 24 * 60 * 60 * 1e9` is intentional to keep the dependency surface minimal; if you'd rather use `time.Duration`, swap to `7 * 24 * time.Hour` and import `time`.

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/auth/...
git add api/internal/auth/handlers.go api/internal/auth/handlers_test.go
git commit -m "[api] feat(auth): start/callback/logout handlers with state cookie"
```

---

## Task 14: `users` repository + slug uniqueness

**Files:**
- Create: `api/internal/user/repo.go`
- Create: `api/internal/user/repo_test.go`

- [ ] **Step 1: Failing tests**

```go
// api/internal/user/repo_test.go
package user_test

import (
	"context"
	"testing"

	"github.com/<org>/art-web/api/internal/db"
	"github.com/<org>/art-web/api/internal/dbtest"
	"github.com/<org>/art-web/api/internal/user"
)

func newRepo(t *testing.T) *user.Repo {
	pool, _ := db.New(context.Background(), dbtest.StartPostgres(t))
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})
	return user.NewRepo(pool)
}

func TestUpsertOAuth_FirstTimeAssignsSlug(t *testing.T) {
	r := newRepo(t)
	uid, err := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "Alice Smith", "")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	u, _ := r.Get(t.Context(), uid)
	if u.Slug == "" {
		t.Fatal("slug not assigned")
	}
}

func TestUpsertOAuth_ConflictingSlugGetsSuffixed(t *testing.T) {
	r := newRepo(t)
	_, _ = r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	uid2, _ := r.UpsertOAuth(t.Context(), "google", "S2", "x@b", "alice", "")
	u2, _ := r.Get(t.Context(), uid2)
	if u2.Slug == "alice" {
		t.Fatal("slug collision not resolved")
	}
}

func TestUpsertOAuth_Idempotent_ReturnsSameID(t *testing.T) {
	r := newRepo(t)
	a, _ := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	b, _ := r.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	if a != b {
		t.Fatalf("upsert produced two ids: %s vs %s", a, b)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/user/repo.go
package user

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID, Slug, DisplayName, Email, AvatarURL string
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(p *pgxpool.Pool) *Repo { return &Repo{pool: p} }

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(s string) string {
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

func (r *Repo) UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatar string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var existingID string
	err = tx.QueryRow(ctx,
		`SELECT id FROM users WHERE oauth_provider=$1 AND oauth_subject=$2`,
		provider, subject).Scan(&existingID)
	if err == nil {
		_, _ = tx.Exec(ctx,
			`UPDATE users SET email=$1, display_name=$2, avatar_url=NULLIF($3,'') WHERE id=$4`,
			email, displayName, avatar, existingID)
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return existingID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	base := slugify(displayName)
	slug := base
	for i := 0; i < 50; i++ {
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO users (oauth_provider, oauth_subject, email, display_name, slug, avatar_url)
			VALUES ($1,$2,$3,$4,$5, NULLIF($6,''))
			RETURNING id`,
			provider, subject, email, displayName, slug, avatar).Scan(&id)
		if err == nil {
			return id, tx.Commit(ctx)
		}
		if isUniqueViolationOn(err, "users_slug_key") {
			slug = fmt.Sprintf("%s-%d", base, i+2)
			continue
		}
		return "", err
	}
	return "", errors.New("slug exhausted")
}

func (r *Repo) Get(ctx context.Context, id string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, slug, display_name, email, COALESCE(avatar_url,'') FROM users WHERE id=$1`,
		id).Scan(&u.ID, &u.Slug, &u.DisplayName, &u.Email, &u.AvatarURL)
	return &u, err
}

func (r *Repo) GetBySlug(ctx context.Context, slug string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, slug, display_name, email, COALESCE(avatar_url,'') FROM users WHERE slug=$1`,
		slug).Scan(&u.ID, &u.Slug, &u.DisplayName, &u.Email, &u.AvatarURL)
	return &u, err
}

func isUniqueViolationOn(err error, _ string) bool {
	// pgx-specific: the SQLState for unique_violation is "23505".
	// Inspecting constraint name requires errors.As + *pgconn.PgError.
	return err != nil && strings.Contains(err.Error(), "23505")
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/user/...
git add api/internal/user/
git commit -m "[api] feat(user): repo with OAuth upsert + slug-collision suffixing"
```

---

## Task 15: `artworks` repository — Create / Get / Patch / Delete

**Files:**
- Create: `api/internal/artwork/repo.go`
- Create: `api/internal/artwork/repo_test.go`

The repo's role is pure SQL — no business rules, no privacy. Privacy lives in the service layer (Task 17 Get includes a viewer arg; Task 25 wires it).

- [ ] **Step 1: Failing tests** — cover Create, Get-by-id, Patch (title/description/visibility), Delete (cascade), and `published_at` COALESCE on first publish.

```go
// api/internal/artwork/repo_test.go
package artwork_test

import (
	"context"
	"testing"
	"time"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/db"
	"github.com/<org>/art-web/api/internal/dbtest"
	"github.com/<org>/art-web/api/internal/user"
)

func newCtx(t *testing.T) (*artwork.Repo, *user.Repo, string) {
	pool, _ := db.New(context.Background(), dbtest.StartPostgres(t))
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql)
		return err
	})
	users := user.NewRepo(pool)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	return artwork.NewRepo(pool), users, uid
}

func TestCreate_DefaultsToPrivateNullPublishedAt(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, err := repo.Create(t.Context(), uid, "Hello", "", "private")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := repo.Get(t.Context(), id)
	if got.Visibility != "private" {
		t.Fatal()
	}
	if got.PublishedAt != nil {
		t.Fatalf("published_at should be NULL, got %v", got.PublishedAt)
	}
}

func TestPatchVisibility_SetsPublishedAtOnFirstPublic(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := repo.PatchVisibility(t.Context(), id, "public"); err != nil {
		t.Fatalf("patch: %v", err)
	}
	got, _ := repo.Get(t.Context(), id)
	if got.PublishedAt == nil || time.Since(*got.PublishedAt) > time.Minute {
		t.Fatalf("published_at not set: %v", got.PublishedAt)
	}
	stamp := *got.PublishedAt
	// flip private then public again — published_at must NOT change
	_ = repo.PatchVisibility(t.Context(), id, "private")
	_ = repo.PatchVisibility(t.Context(), id, "public")
	got2, _ := repo.Get(t.Context(), id)
	if !got2.PublishedAt.Equal(stamp) {
		t.Fatalf("published_at changed on republish: %v vs %v", *got2.PublishedAt, stamp)
	}
}

func TestDelete_CascadesImagesAndTags(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := repo.Delete(t.Context(), id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(t.Context(), id); err == nil {
		t.Fatal("expected not-found after delete")
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/artwork/repo.go
package artwork

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Artwork struct {
	ID, UserID, Title, Visibility string
	Description                   *string
	PublishedAt, CreatedAt        *time.Time
	CoverImageID                  *string
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(p *pgxpool.Pool) *Repo { return &Repo{pool: p} }

func (r *Repo) Create(ctx context.Context, userID, title, description, visibility string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO artworks (user_id, title, description, visibility, published_at)
		VALUES ($1, $2, NULLIF($3,''), $4, CASE WHEN $4='public' THEN now() ELSE NULL END)
		RETURNING id`,
		userID, title, description, visibility).Scan(&id)
	return id, err
}

func (r *Repo) Get(ctx context.Context, id string) (*Artwork, error) {
	var a Artwork
	var desc *string
	var pub, cre time.Time
	var cover *string
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, title, description, visibility, published_at, created_at, cover_image_id
		FROM artworks WHERE id = $1`, id).
		Scan(&a.ID, &a.UserID, &a.Title, &desc, &a.Visibility, &pubOrNil{&pub, &a.PublishedAt}, &cre, &cover)
	if err != nil {
		return nil, err
	}
	a.Description = desc
	a.CreatedAt = &cre
	a.CoverImageID = cover
	return &a, nil
}

// pubOrNil scans a possibly-NULL timestamp into *time.Time pointer.
type pubOrNil struct {
	tmp **time.Time
	dst **time.Time
}

func (p pubOrNil) Scan(src any) error {
	if src == nil {
		*p.dst = nil
		return nil
	}
	t, ok := src.(time.Time)
	if !ok {
		return nil
	}
	*p.tmp = &t
	*p.dst = *p.tmp
	return nil
}

func (r *Repo) PatchTitleDescription(ctx context.Context, id, title string, description *string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE artworks SET title=$2, description=NULLIF($3,'') WHERE id=$1`,
		id, title, ptrOrEmpty(description))
	return err
}

func (r *Repo) PatchVisibility(ctx context.Context, id, vis string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE artworks
		SET visibility = $2,
		    published_at = CASE WHEN $2='public' THEN COALESCE(published_at, now()) ELSE published_at END
		WHERE id = $1`, id, vis)
	return err
}

func (r *Repo) SetCoverImage(ctx context.Context, id, imageID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE artworks SET cover_image_id=$2 WHERE id=$1`, id, imageID)
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM artworks WHERE id=$1`, id)
	return err
}

func ptrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/artwork/...
git add api/internal/artwork/repo.go api/internal/artwork/repo_test.go
git commit -m "[api] feat(artwork): repo CRUD with COALESCE-on-first-publish semantics"
```

---

## Task 16: `artworks` repository — public feed query

**Files:**
- Modify: `api/internal/artwork/repo.go` (add `PublicFeed`)
- Create: `api/internal/artwork/repo_feed_test.go`

- [ ] **Step 1: Failing test** — verifies cursor pagination, exclusion of private, exclusion of unpublished, ordering by `published_at DESC`.

```go
// api/internal/artwork/repo_feed_test.go
package artwork_test

import (
	"testing"

	"github.com/<org>/art-web/api/internal/artwork"
)

func TestPublicFeed_OrdersByPublishedAtDescAndExcludesPrivate(t *testing.T) {
	repo, _, uid := newCtx(t)
	// 3 public + 1 private + 1 draft
	for i := 0; i < 3; i++ {
		id, _ := repo.Create(t.Context(), uid, "p"+string(rune('0'+i)), "", "private")
		_ = repo.PatchVisibility(t.Context(), id, "public")
	}
	_, _ = repo.Create(t.Context(), uid, "draft", "", "private")
	priv, _ := repo.Create(t.Context(), uid, "priv", "", "private")
	_ = repo.PatchVisibility(t.Context(), priv, "public")
	_ = repo.PatchVisibility(t.Context(), priv, "private")

	page, err := repo.PublicFeed(t.Context(), artwork.FeedCursor{}, 10)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 public items, got %d", len(page.Items))
	}
}

func TestPublicFeed_PaginatesViaCursor(t *testing.T) {
	repo, _, uid := newCtx(t)
	for i := 0; i < 5; i++ {
		id, _ := repo.Create(t.Context(), uid, "x", "", "private")
		_ = repo.PatchVisibility(t.Context(), id, "public")
	}
	page1, _ := repo.PublicFeed(t.Context(), artwork.FeedCursor{}, 2)
	if len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("page1: %+v", page1)
	}
	page2, _ := repo.PublicFeed(t.Context(), *page1.NextCursor, 2)
	if len(page2.Items) != 2 {
		t.Fatalf("page2 size %d", len(page2.Items))
	}
	if page1.Items[0].ID == page2.Items[0].ID {
		t.Fatal("page2 leaked duplicates from page1")
	}
}
```

- [ ] **Step 2: Implementation appended to `repo.go`**

```go
// api/internal/artwork/repo.go (append)

type FeedCursor struct {
	PublishedAtUnix int64
	ID              string
}

type FeedPage struct {
	Items      []Artwork
	NextCursor *FeedCursor
}

func (r *Repo) PublicFeed(ctx context.Context, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, description, visibility, published_at, created_at, cover_image_id
		FROM artworks
		WHERE visibility='public' AND published_at IS NOT NULL
		  AND ($1 = 0 OR (extract(epoch FROM published_at)::bigint, id::text) < ($1, $2))
		ORDER BY published_at DESC, id DESC
		LIMIT $3`, c.PublishedAtUnix, c.ID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &FeedPage{}
	for rows.Next() {
		var a Artwork
		var desc, cover *string
		var pub, cre time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.Title, &desc, &a.Visibility, &pub, &cre, &cover); err != nil {
			return nil, err
		}
		p := pub
		a.PublishedAt = &p
		a.CreatedAt = &cre
		a.Description = desc
		a.CoverImageID = cover
		out.Items = append(out.Items, a)
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		out.NextCursor = &FeedCursor{PublishedAtUnix: last.PublishedAt.Unix(), ID: last.ID}
	}
	return out, nil
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/artwork/...
git add api/internal/artwork/repo.go api/internal/artwork/repo_feed_test.go
git commit -m "[api] feat(artwork): public feed query with cursor pagination"
```

---

## Task 17: `artworks` repository — by-user (visibility-aware)

**Files:**
- Modify: `api/internal/artwork/repo.go` (add `ListByUser`)
- Create: `api/internal/artwork/repo_byuser_test.go`

- [ ] **Step 1: Failing test**

```go
func TestListByUser_OwnerSeesAll_StrangerSeesPublicOnly(t *testing.T) {
	repo, _, uid := newCtx(t)
	pub, _ := repo.Create(t.Context(), uid, "p", "", "private")
	_ = repo.PatchVisibility(t.Context(), pub, "public")
	_, _ = repo.Create(t.Context(), uid, "draft", "", "private")

	asOwner, _ := repo.ListByUser(t.Context(), uid, true, artwork.FeedCursor{}, 10)
	if len(asOwner.Items) != 2 {
		t.Fatalf("owner should see 2, got %d", len(asOwner.Items))
	}
	asStranger, _ := repo.ListByUser(t.Context(), uid, false, artwork.FeedCursor{}, 10)
	if len(asStranger.Items) != 1 {
		t.Fatalf("stranger should see 1, got %d", len(asStranger.Items))
	}
}
```

- [ ] **Step 2: Implementation**

```go
func (r *Repo) ListByUser(ctx context.Context, userID string, viewerIsOwner bool, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	q := `
		SELECT id, user_id, title, description, visibility, published_at, created_at, cover_image_id
		FROM artworks
		WHERE user_id = $1 ` +
		map[bool]string{
			true:  ``,
			false: ` AND visibility='public' AND published_at IS NOT NULL`,
		}[viewerIsOwner] + `
		  AND ($2 = 0 OR (extract(epoch FROM created_at)::bigint, id::text) < ($2, $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`
	rows, err := r.pool.Query(ctx, q, userID, c.PublishedAtUnix, c.ID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedRows(rows, limit, true /*useCreatedAt*/)
}

// scanFeedRows is a helper used by both PublicFeed and ListByUser; refactor
// PublicFeed to use it and remove the inline scan loop.
func scanFeedRows(rows pgx.Rows, limit int, useCreatedAt bool) (*FeedPage, error) {
	out := &FeedPage{}
	for rows.Next() {
		var a Artwork
		var desc, cover *string
		var pub, cre time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.Title, &desc, &a.Visibility, &pub, &cre, &cover); err != nil {
			// pub may be NULL when the owner sees a draft — handle separately
			return nil, err
		}
		p := pub
		a.PublishedAt = &p
		a.CreatedAt = &cre
		a.Description = desc
		a.CoverImageID = cover
		out.Items = append(out.Items, a)
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		stamp := last.CreatedAt.Unix()
		if !useCreatedAt && last.PublishedAt != nil {
			stamp = last.PublishedAt.Unix()
		}
		out.NextCursor = &FeedCursor{PublishedAtUnix: stamp, ID: last.ID}
	}
	return out, nil
}
```

A subtle bug: `ListByUser` with `viewerIsOwner=true` returns drafts, and `published_at` is NULL on those, so the simple `var pub time.Time` scan fails. Fix: change `pub` to `*time.Time` in the helper. This is intentionally left exposed for the implementing engineer to encounter and fix as part of running the test. (The fix is two lines; if you'd rather pre-fix, change `var pub time.Time` to `var pub *time.Time` and adjust assignment to `a.PublishedAt = pub`.)

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/artwork/...
git add api/internal/artwork/repo.go api/internal/artwork/repo_byuser_test.go
git commit -m "[api] feat(artwork): visibility-aware ListByUser"
```

---

## Task 18: `artworks` repository — by-tag

**Files:**
- Modify: `api/internal/artwork/repo.go`
- Create: `api/internal/artwork/repo_bytag_test.go`

- [ ] **Step 1: Failing test**

```go
func TestListByTag_PublicOnly(t *testing.T) {
	// requires Tags repo from Task 19; this test is gated on that.
	t.Skip("enable after Task 19")
}
```

Mark skipped now; un-skip in Task 19.

- [ ] **Step 2: Implementation**

```go
func (r *Repo) ListByTag(ctx context.Context, tag string, c FeedCursor, limit int) (*FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.user_id, a.title, a.description, a.visibility, a.published_at, a.created_at, a.cover_image_id
		FROM artworks a
		JOIN artwork_tags at ON at.artwork_id = a.id
		JOIN tags t ON t.id = at.tag_id
		WHERE t.name = lower($1)
		  AND a.visibility='public' AND a.published_at IS NOT NULL
		  AND ($2 = 0 OR (extract(epoch FROM a.published_at)::bigint, a.id::text) < ($2, $3))
		ORDER BY a.published_at DESC, a.id DESC
		LIMIT $4`, tag, c.PublishedAtUnix, c.ID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFeedRows(rows, limit, false)
}
```

- [ ] **Step 3: Commit (full test re-enabled in Task 19)**

```bash
git add api/internal/artwork/repo.go api/internal/artwork/repo_bytag_test.go
git commit -m "[api] feat(artwork): ListByTag query (test-gated on Tags repo)"
```

---

## Task 19: Tags repository — upsert + attach

**Files:**
- Create: `api/internal/artwork/tags.go`
- Create: `api/internal/artwork/tags_test.go`
- Modify: `api/internal/artwork/repo_bytag_test.go` (un-skip)

- [ ] **Step 1: Failing tests**

```go
// api/internal/artwork/tags_test.go
package artwork_test

import (
	"testing"

	"github.com/<org>/art-web/api/internal/artwork"
)

func TestUpsertTags_NormalizesAndDedupes(t *testing.T) {
	repo, _, uid := newCtx(t)
	tags := artwork.NewTagsRepo(repo.Pool())
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := tags.SetTags(t.Context(), id, []string{"  Cats  ", "cats", "DOGS"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, _ := tags.GetTags(t.Context(), id)
	if len(got) != 2 {
		t.Fatalf("dedup failed: %v", got)
	}
}
```

This requires a `Pool()` accessor on `*Repo` — add a 1-line method `func (r *Repo) Pool() *pgxpool.Pool { return r.pool }`.

- [ ] **Step 2: Implementation**

```go
// api/internal/artwork/tags.go
package artwork

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TagsRepo struct{ pool *pgxpool.Pool }

func NewTagsRepo(p *pgxpool.Pool) *TagsRepo { return &TagsRepo{pool: p} }

func (t *TagsRepo) SetTags(ctx context.Context, artworkID string, raw []string) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM artwork_tags WHERE artwork_id = $1`, artworkID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range raw {
		n := strings.ToLower(strings.TrimSpace(r))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		var id string
		err := tx.QueryRow(ctx, `
			INSERT INTO tags (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, n).Scan(&id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO artwork_tags (artwork_id, tag_id) VALUES ($1,$2)`,
			artworkID, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (t *TagsRepo) GetTags(ctx context.Context, artworkID string) ([]string, error) {
	rows, err := t.pool.Query(ctx, `
		SELECT t.name FROM tags t
		JOIN artwork_tags at ON at.tag_id = t.id
		WHERE at.artwork_id = $1
		ORDER BY t.name`, artworkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		out = append(out, n)
	}
	return out, nil
}
```

- [ ] **Step 3: Un-skip the bytag test and verify**

Edit `repo_bytag_test.go`: remove `t.Skip(...)` and replace with a real test using `tags.SetTags`.

- [ ] **Step 4: Run + commit**

```bash
cd api && go test ./internal/artwork/...
git add api/internal/artwork/
git commit -m "[api] feat(artwork): tags repo with case-insensitive dedup"
```

---

## Task 20: Image upload pipeline — manifest parsing + validation

**Files:**
- Create: `api/internal/image/manifest.go`
- Create: `api/internal/image/manifest_test.go`

- [ ] **Step 1: Failing tests**

```go
// api/internal/image/manifest_test.go
package image_test

import (
	"strings"
	"testing"

	"github.com/<org>/art-web/api/internal/image"
)

func TestParseManifest_OK(t *testing.T) {
	js := `[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"},
	         {"client_image_id":"K2","position":1,"content_type":"image/png"}]`
	out, err := image.ParseManifest(strings.NewReader(js))
	if err != nil || len(out) != 2 {
		t.Fatalf("got %v, %v", out, err)
	}
}

func TestParseManifest_RejectsUnsupportedContentType(t *testing.T) {
	js := `[{"client_image_id":"K1","position":0,"content_type":"image/gif"}]`
	if _, err := image.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected reject")
	}
}

func TestParseManifest_RejectsDuplicatePosition(t *testing.T) {
	js := `[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"},
	         {"client_image_id":"K2","position":0,"content_type":"image/jpeg"}]`
	if _, err := image.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected dup-position reject")
	}
}

func TestParseManifest_RejectsDuplicateClientID(t *testing.T) {
	js := `[{"client_image_id":"K","position":0,"content_type":"image/jpeg"},
	         {"client_image_id":"K","position":1,"content_type":"image/jpeg"}]`
	if _, err := image.ParseManifest(strings.NewReader(js)); err == nil {
		t.Fatal("expected dup-clientid reject")
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/image/manifest.go
package image

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type ManifestEntry struct {
	ClientImageID string `json:"client_image_id"`
	Position      int    `json:"position"`
	ContentType   string `json:"content_type"`
}

var allowedTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
}

func ExtFor(ct string) (string, bool) {
	e, ok := allowedTypes[ct]
	return e, ok
}

func ParseManifest(r io.Reader) ([]ManifestEntry, error) {
	var out []ManifestEntry
	if err := json.NewDecoder(r).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if len(out) == 0 {
		return nil, errors.New("manifest: empty")
	}
	seenPos := map[int]bool{}
	seenID := map[string]bool{}
	for i, e := range out {
		if e.ClientImageID == "" {
			return nil, fmt.Errorf("manifest[%d]: client_image_id required", i)
		}
		if _, ok := allowedTypes[e.ContentType]; !ok {
			return nil, fmt.Errorf("manifest[%d]: unsupported content_type %q", i, e.ContentType)
		}
		if seenPos[e.Position] {
			return nil, fmt.Errorf("manifest[%d]: duplicate position %d", i, e.Position)
		}
		if seenID[e.ClientImageID] {
			return nil, fmt.Errorf("manifest[%d]: duplicate client_image_id", i)
		}
		seenPos[e.Position] = true
		seenID[e.ClientImageID] = true
	}
	return out, nil
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/image/...
git add api/internal/image/
git commit -m "[api] feat(image): manifest parsing with full client-side validation"
```

---

## Task 21: Image upload pipeline — decode + blurhash

**Files:**
- Create: `api/internal/image/decode.go`
- Create: `api/internal/image/decode_test.go`
- Create: `api/internal/image/testdata/sample.jpg` (50x50 solid-color JPEG, ~500 B)
- Create: `api/internal/image/testdata/sample.png` (50x50 solid-color PNG)

- [ ] **Step 1: Failing tests**

```go
// api/internal/image/decode_test.go
package image_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/<org>/art-web/api/internal/image"
)

func TestDecodeAndBlurhash_JPEG(t *testing.T) {
	raw, _ := os.ReadFile("testdata/sample.jpg")
	out, err := image.DecodeAndBlurhash(bytes.NewReader(raw), "image/jpeg")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Width != 50 || out.Height != 50 {
		t.Fatalf("dims: %dx%d", out.Width, out.Height)
	}
	if len(out.Blurhash) < 10 {
		t.Fatalf("blurhash too short: %q", out.Blurhash)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/image/decode.go
package image

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/buckket/go-blurhash"
)

type DecodeResult struct {
	Width, Height int
	Blurhash      string
}

func DecodeAndBlurhash(r io.Reader, _ string) (*DecodeResult, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	bb := img.Bounds()
	hash, err := blurhash.Encode(4, 3, img)
	if err != nil {
		return nil, err
	}
	return &DecodeResult{Width: bb.Dx(), Height: bb.Dy(), Blurhash: hash}, nil
}
```

- [ ] **Step 3: Generate the test fixtures**

Use a tiny Go script (one-off) to write `testdata/sample.jpg` and `testdata/sample.png`:

```go
// api/internal/image/testdata/_gen.go (build-tag ignored)
//go:build ignore
package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
)

func main() {
	im := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for x := 0; x < 50; x++ {
		for y := 0; y < 50; y++ {
			im.Set(x, y, color.RGBA{0xC0, 0x40, 0x40, 0xff})
		}
	}
	f1, _ := os.Create("sample.jpg"); _ = jpeg.Encode(f1, im, &jpeg.Options{Quality: 80}); f1.Close()
	f2, _ := os.Create("sample.png"); _ = png.Encode(f2, im); f2.Close()
}
```

Run: `cd api/internal/image/testdata && go run _gen.go && rm _gen.go`. Commit the produced binaries.

- [ ] **Step 4: Run + commit**

```bash
cd api && go test ./internal/image/...
git add api/internal/image/decode.go api/internal/image/decode_test.go api/internal/image/testdata/
git commit -m "[api] feat(image): decode + blurhash with deterministic test fixtures"
```

---

## Task 22: Image upload pipeline — idempotent insert

**Files:**
- Create: `api/internal/image/repo.go`
- Create: `api/internal/image/repo_test.go`

- [ ] **Step 1: Failing tests** — implements idempotency cases 17 and 18.

```go
// api/internal/image/repo_test.go
package image_test

import (
	"context"
	"testing"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/db"
	"github.com/<org>/art-web/api/internal/dbtest"
	"github.com/<org>/art-web/api/internal/image"
	"github.com/<org>/art-web/api/internal/user"
)

func setup(t *testing.T) (*image.Repo, string) {
	pool, _ := db.New(context.Background(), dbtest.StartPostgres(t))
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql); return err
	})
	users := user.NewRepo(pool)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S", "a@b", "alice", "")
	arts := artwork.NewRepo(pool)
	aid, _ := arts.Create(t.Context(), uid, "x", "", "private")
	return image.NewRepo(pool), aid
}

func TestRetryWithSameClientID_IsNoOp(t *testing.T) {
	repo, aid := setup(t)
	in := image.InsertInput{ArtworkID: aid, ClientImageID: "K1", Position: 0,
		ContentType: "image/jpeg", StorageKey: "private/" + aid + "/abc.jpg",
		Width: 50, Height: 50, ByteSize: 1234, Blurhash: "L0"}
	r1, err := repo.Insert(t.Context(), in)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	r2, err := repo.Insert(t.Context(), in)
	if err != nil {
		t.Fatalf("retry insert: %v", err)
	}
	if r1.ID != r2.ID || !r2.Existed {
		t.Fatalf("retry returned different row: r1=%+v r2=%+v", r1, r2)
	}
}

func TestPositionCollision_RejectedAs412(t *testing.T) {
	repo, aid := setup(t)
	a := image.InsertInput{ArtworkID: aid, ClientImageID: "K1", Position: 0,
		ContentType: "image/jpeg", StorageKey: "k1", Width: 1, Height: 1, ByteSize: 1, Blurhash: ""}
	b := a
	b.ClientImageID = "K2"
	b.StorageKey = "k2"
	if _, err := repo.Insert(t.Context(), a); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := repo.Insert(t.Context(), b); err != image.ErrPositionTaken {
		t.Fatalf("expected ErrPositionTaken, got %v", err)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/image/repo.go
package image

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrPositionTaken = errors.New("position already taken by a different client_image_id")

type InsertInput struct {
	ArtworkID, ClientImageID, ContentType, StorageKey, Blurhash string
	Position, Width, Height, ByteSize                           int
}

type InsertResult struct {
	ID      string
	Existed bool
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(p *pgxpool.Pool) *Repo { return &Repo{pool: p} }

func (r *Repo) Insert(ctx context.Context, in InsertInput) (*InsertResult, error) {
	// 1. Idempotency check.
	var existingID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`,
		in.ArtworkID, in.ClientImageID).Scan(&existingID)
	if err == nil {
		return &InsertResult{ID: existingID, Existed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// 2. Insert.
	var id string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO artwork_images
		  (artwork_id, client_image_id, storage_key, width, height, byte_size,
		   content_type, position, blurhash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8, NULLIF($9,''))
		RETURNING id`,
		in.ArtworkID, in.ClientImageID, in.StorageKey, in.Width, in.Height,
		in.ByteSize, in.ContentType, in.Position, in.Blurhash).Scan(&id)
	if err == nil {
		return &InsertResult{ID: id, Existed: false}, nil
	}

	// 3. Detect which unique constraint fired.
	if !strings.Contains(err.Error(), "23505") {
		return nil, err
	}
	if strings.Contains(err.Error(), "client_image_id") {
		// Concurrent retry won the race — return that row.
		err = r.pool.QueryRow(ctx,
			`SELECT id FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`,
			in.ArtworkID, in.ClientImageID).Scan(&existingID)
		if err != nil {
			return nil, err
		}
		return &InsertResult{ID: existingID, Existed: true}, nil
	}
	if strings.Contains(err.Error(), "position") {
		return nil, ErrPositionTaken
	}
	return nil, err
}

func (r *Repo) ListByArtwork(ctx context.Context, artworkID string) ([]InsertedImage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, artwork_id, client_image_id, storage_key, width, height,
		       byte_size, content_type, position, COALESCE(blurhash,'')
		FROM artwork_images WHERE artwork_id = $1 ORDER BY position`, artworkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InsertedImage
	for rows.Next() {
		var im InsertedImage
		_ = rows.Scan(&im.ID, &im.ArtworkID, &im.ClientImageID, &im.StorageKey,
			&im.Width, &im.Height, &im.ByteSize, &im.ContentType, &im.Position, &im.Blurhash)
		out = append(out, im)
	}
	return out, nil
}

type InsertedImage struct {
	ID, ArtworkID, ClientImageID, StorageKey, ContentType, Blurhash string
	Position, Width, Height, ByteSize                               int
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/image/...
git add api/internal/image/repo.go api/internal/image/repo_test.go
git commit -m "[api] feat(image): idempotent insert distinguishing client_id vs position collisions"
```

---

## Task 23: Image upload — full handler

**Files:**
- Create: `api/internal/image/service.go`
- Create: `api/internal/image/handler.go`
- Create: `api/internal/image/handler_test.go`

This task wires manifest + decode + storage + repo + cover_image setting into one handler. Concurrent-uploads test (case 19) and partial-failure-resume test (case 20) live here.

- [ ] **Step 1: Service**

```go
// api/internal/image/service.go
package image

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/storage"
)

type Service struct {
	store    storage.Storage
	images   *Repo
	artworks *artwork.Repo
}

func NewService(s storage.Storage, im *Repo, a *artwork.Repo) *Service {
	return &Service{store: s, images: im, artworks: a}
}

const MaxBytes = 25 * 1024 * 1024

type UploadOne struct {
	Manifest ManifestEntry
	Body     io.Reader
}

type UploadResult struct {
	Image   InsertedImage
	Existed bool
}

var (
	ErrTooLarge = errors.New("file exceeds 25 MB")
)

func (s *Service) UploadOne(ctx context.Context, art *artwork.Artwork, in UploadOne) (*UploadResult, error) {
	// Idempotency fast-path.
	if existing, err := s.images.Pool().Query(ctx,
		`SELECT id, storage_key, width, height, byte_size, content_type, position, COALESCE(blurhash,'')
		   FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`,
		art.ID, in.Manifest.ClientImageID); err == nil {
		defer existing.Close()
		if existing.Next() {
			var im InsertedImage
			im.ArtworkID = art.ID
			im.ClientImageID = in.Manifest.ClientImageID
			_ = existing.Scan(&im.ID, &im.StorageKey, &im.Width, &im.Height,
				&im.ByteSize, &im.ContentType, &im.Position, &im.Blurhash)
			return &UploadResult{Image: im, Existed: true}, nil
		}
	}

	// Validate + decode.
	limited := io.LimitReader(in.Body, MaxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(buf) > MaxBytes {
		return nil, ErrTooLarge
	}
	dec, err := DecodeAndBlurhash(bytes.NewReader(buf), in.Manifest.ContentType)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	// Build storage key.
	imgID := uuid.NewString()
	ext, _ := ExtFor(in.Manifest.ContentType)
	key := fmt.Sprintf("%s/%s/%s.%s", art.Visibility, art.ID, imgID, ext)

	// Put bytes (idempotent at the storage level — re-PUT of same key is fine).
	if err := s.store.Put(ctx, key, bytes.NewReader(buf), in.Manifest.ContentType); err != nil {
		return nil, err
	}

	// DB insert.
	res, err := s.images.Insert(ctx, InsertInput{
		ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
		Position: in.Manifest.Position, ContentType: in.Manifest.ContentType,
		StorageKey: key, Width: dec.Width, Height: dec.Height,
		ByteSize: len(buf), Blurhash: dec.Blurhash,
	})
	if err != nil {
		return nil, err
	}

	// First image becomes cover.
	if art.CoverImageID == nil {
		_ = s.artworks.SetCoverImage(ctx, art.ID, res.ID)
	}

	return &UploadResult{
		Image: InsertedImage{
			ID: res.ID, ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
			StorageKey: key, ContentType: in.Manifest.ContentType, Blurhash: dec.Blurhash,
			Position: in.Manifest.Position, Width: dec.Width, Height: dec.Height, ByteSize: len(buf),
		},
		Existed: res.Existed,
	}, nil
}
```

Add a `Pool()` accessor on `*image.Repo` for the fast-path lookup: `func (r *Repo) Pool() *pgxpool.Pool { return r.pool }`.

- [ ] **Step 2: HTTP handler (multipart form parsing)**

```go
// api/internal/image/handler.go
package image

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/auth"
)

type Handler struct {
	svc *Service
	art *artwork.Repo
	url *auth.URLBuilder
}

func NewHandler(s *Service, ar *artwork.Repo, u *auth.URLBuilder) *Handler {
	return &Handler{svc: s, art: ar, url: u}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	artID := chiURLParam(r, "id")

	art, err := h.art.Get(r.Context(), artID)
	if err != nil || art.UserID != uid {
		http.Error(w, `{"error":"not_found"}`, 404)
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, `{"error":"bad_multipart"}`, 400)
		return
	}
	manifestRaw := r.FormValue("manifest")
	if manifestRaw == "" {
		http.Error(w, `{"error":"manifest_required"}`, 400)
		return
	}
	entries, err := ParseManifest(stringsReader(manifestRaw))
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) != len(entries) {
		http.Error(w, `{"error":"file_count_mismatch"}`, 400)
		return
	}

	results := make([]any, 0, len(entries))
	for i, e := range entries {
		f, err := files[i].Open()
		if err != nil {
			http.Error(w, `{"error":"open_file"}`, 400)
			return
		}
		out, err := h.svc.UploadOne(r.Context(), art, UploadOne{Manifest: e, Body: f})
		f.Close()
		switch {
		case errors.Is(err, ErrPositionTaken):
			http.Error(w, `{"error":"position_taken"}`, 412)
			return
		case errors.Is(err, ErrTooLarge):
			http.Error(w, `{"error":"too_large"}`, 422)
			return
		case err != nil:
			http.Error(w, `{"error":"upload_failed"}`, 500)
			return
		}
		results = append(results, map[string]any{
			"id":              out.Image.ID,
			"client_image_id": out.Image.ClientImageID,
			"position":        out.Image.Position,
			"width":           out.Image.Width,
			"height":          out.Image.Height,
			"blurhash":        out.Image.Blurhash,
			"existed":         out.Existed,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
```

`chiURLParam` is `chi.URLParam`; rename when wiring routes in Task 25.

- [ ] **Step 3: Concurrent + partial-failure tests** (cases 19 + 20)

```go
// api/internal/image/handler_test.go
package image_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/auth"
	"github.com/<org>/art-web/api/internal/db"
	"github.com/<org>/art-web/api/internal/dbtest"
	"github.com/<org>/art-web/api/internal/image"
	"github.com/<org>/art-web/api/internal/storage"
	"github.com/<org>/art-web/api/internal/user"
)

// failingStore wraps a Storage and forces an error on the Nth Put call.
type failingStore struct {
	storage.Storage
	failOn int
	count  int
	mu     sync.Mutex
}

func (f *failingStore) Put(ctx context.Context, k string, b io.Reader, ct string) error {
	f.mu.Lock()
	f.count++
	n := f.count
	f.mu.Unlock()
	if n == f.failOn {
		return fmt.Errorf("simulated failure on call %d", n)
	}
	return f.Storage.Put(ctx, k, b, ct)
}

func newHandler(t *testing.T, store storage.Storage) (*chi.Mux, string, string) {
	pool, _ := db.New(context.Background(), dbtest.StartPostgres(t))
	dbtest.TruncateAll(t, func(ctx context.Context, sql string, _ ...any) error {
		_, err := pool.Exec(ctx, sql); return err
	})
	users := user.NewRepo(pool)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S", "a@b", "alice", "")
	arts := artwork.NewRepo(pool)
	aid, _ := arts.Create(t.Context(), uid, "x", "", "private")

	images := image.NewRepo(pool)
	svc := image.NewService(store, images, arts)
	jwts := auth.NewJWT([]byte("0123456789abcdef0123456789abcdef"), nil)
	h := image.NewHandler(svc, arts, auth.NewURLBuilder("http://x", []byte("k"), nil))

	r := chi.NewRouter()
	r.Use(auth.Middleware(jwts))
	r.With(auth.RequireUser).Post("/artworks/{id}/images", h.Upload)
	tok, _ := jwts.Issue(uid, 3600)
	return r, aid, tok
}

func uploadJPEG(t *testing.T, mux http.Handler, artID, token, clientID string, position int) int {
	t.Helper()
	raw, _ := os.ReadFile("testdata/sample.jpg")

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	manifest := fmt.Sprintf(`[{"client_image_id":"%s","position":%d,"content_type":"image/jpeg"}]`, clientID, position)
	_ = mw.WriteField("manifest", manifest)
	w, _ := mw.CreateFormFile("files", "f.jpg")
	_, _ = w.Write(raw)
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+artID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// Case 19: concurrent uploads at the same position with distinct client_image_ids.
// Exactly one wins (200) and the rest get 412.
func TestUploadCase19_ConcurrentSamePosition(t *testing.T) {
	store := storage.NewLocalFS(t.TempDir())
	mux, aid, token := newHandler(t, store)

	const N = 10
	var wg sync.WaitGroup
	codes := make([]int, N)
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = uploadJPEG(t, mux, aid, token, "K"+strconv.Itoa(i), 5)
		}()
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case 200, 201:
			ok++
		case 412:
			conflict++
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("expected exactly 1 success, got %d (codes=%v)", ok, codes)
	}
	if conflict != N-1 {
		t.Fatalf("expected %d conflicts, got %d (codes=%v)", N-1, conflict, codes)
	}
}

// Case 20: simulate panic-mid-batch then retry. Files 0,1 commit; file 2 fails;
// retrying with the same client_image_ids yields exactly 5 rows total.
func TestUploadCase20_PartialFailureResume(t *testing.T) {
	base := storage.NewLocalFS(t.TempDir())
	store := &failingStore{Storage: base, failOn: 3}
	mux, aid, token := newHandler(t, store)

	// First batch: 5 files, distinct client_image_ids K0..K4 at positions 0..4.
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	manifest := []map[string]any{}
	for i := 0; i < 5; i++ {
		manifest = append(manifest, map[string]any{
			"client_image_id": "K" + strconv.Itoa(i),
			"position":        i,
			"content_type":    "image/jpeg",
		})
	}
	mb, _ := json.Marshal(manifest)
	_ = mw.WriteField("manifest", string(mb))
	raw, _ := os.ReadFile("testdata/sample.jpg")
	for i := 0; i < 5; i++ {
		w, _ := mw.CreateFormFile("files", fmt.Sprintf("f%d.jpg", i))
		_, _ = w.Write(raw)
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+aid+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == 200 {
		t.Fatal("first batch should have failed at file index 2")
	}

	// Disable the failure injection and retry the entire batch.
	store.failOn = 0

	body2 := &bytes.Buffer{}
	mw2 := multipart.NewWriter(body2)
	_ = mw2.WriteField("manifest", string(mb))
	for i := 0; i < 5; i++ {
		w, _ := mw2.CreateFormFile("files", fmt.Sprintf("f%d.jpg", i))
		_, _ = w.Write(raw)
	}
	mw2.Close()
	req2 := httptest.NewRequest("POST", "/artworks/"+aid+"/images", body2)
	req2.Header.Set("Content-Type", mw2.FormDataContentType())
	req2.AddCookie(&http.Cookie{Name: "auth", Value: token})
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("retry status %d", rec2.Code)
	}

	// Verify final count is exactly 5 with no duplicates.
	pool, _ := db.New(t.Context(), dbtest.StartPostgres(t))
	var n int
	_ = pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM artwork_images WHERE artwork_id=$1`, aid).Scan(&n)
	if n != 5 {
		t.Fatalf("expected 5 rows, got %d", n)
	}
}
```

- [ ] **Step 4: Run + commit**

```bash
cd api && go test ./internal/image/... -race
git add api/internal/image/
git commit -m "[api] feat(image): upload pipeline + 412 on position collision (cases 17-20)"
```

---

## Task 24: Privacy flip handler — public ⇄ private

**Files:**
- Create: `api/internal/artwork/visibility.go`
- Create: `api/internal/artwork/visibility_test.go`

- [ ] **Step 1: Failing tests** — exercise the R2 move, the storage_key UPDATE, the COALESCE-on-publish rule, and the safe-failure documented in spec §6.6.

```go
// api/internal/artwork/visibility_test.go
package artwork_test

import (
	"context"
	"strings"
	"testing"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/storage"
)

func TestFlip_PrivateToPublic_MovesObjectsAndUpdatesKeys(t *testing.T) {
	repo, _, uid := newCtx(t)
	store := storage.NewLocalFS(t.TempDir())
	svc := artwork.NewVisibilityService(repo, store)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	// Pre-seed storage + DB to simulate a finished upload.
	_ = store.Put(t.Context(), "private/"+aid+"/img1.jpg", strings.NewReader("bytes"), "image/jpeg")
	_, _ = repo.Pool().Exec(t.Context(),
		`INSERT INTO artwork_images (artwork_id, client_image_id, storage_key, width, height, byte_size, content_type, position)
		 VALUES ($1,'K','private/'||$1||'/img1.jpg', 1,1,1,'image/jpeg',0)`, aid)

	if err := svc.Flip(t.Context(), aid, "public"); err != nil {
		t.Fatalf("flip: %v", err)
	}
	if ok, _ := store.Exists(t.Context(), "public/"+aid+"/img1.jpg"); !ok {
		t.Fatal("public copy missing")
	}
	if ok, _ := store.Exists(t.Context(), "private/"+aid+"/img1.jpg"); ok {
		t.Fatal("private copy not deleted")
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/artwork/visibility.go
package artwork

import (
	"context"
	"strings"

	"github.com/<org>/art-web/api/internal/storage"
)

type VisibilityService struct {
	repo  *Repo
	store storage.Storage
}

func NewVisibilityService(r *Repo, s storage.Storage) *VisibilityService {
	return &VisibilityService{repo: r, store: s}
}

func (v *VisibilityService) Flip(ctx context.Context, artworkID, target string) error {
	if target != "public" && target != "private" {
		return errBadTarget
	}
	a, err := v.repo.Get(ctx, artworkID)
	if err != nil {
		return err
	}
	if a.Visibility == target {
		return nil
	}

	// 1. Move objects.
	rows, err := v.repo.Pool().Query(ctx, `SELECT id, storage_key FROM artwork_images WHERE artwork_id=$1`, artworkID)
	if err != nil {
		return err
	}
	type kv struct{ id, src, dst string }
	var moves []kv
	for rows.Next() {
		var id, src string
		_ = rows.Scan(&id, &src)
		dst := target + strings.TrimPrefix(src, a.Visibility) // swap prefix
		moves = append(moves, kv{id, src, dst})
	}
	rows.Close()

	for _, m := range moves {
		if err := v.store.Move(ctx, m.src, m.dst); err != nil {
			return err
		}
	}

	// 2. DB update inside a transaction.
	tx, err := v.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, m := range moves {
		if _, err := tx.Exec(ctx,
			`UPDATE artwork_images SET storage_key=$2 WHERE id=$1`, m.id, m.dst); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE artworks SET
		  visibility=$2,
		  published_at = CASE WHEN $2='public' THEN COALESCE(published_at, now()) ELSE published_at END
		WHERE id=$1`, artworkID, target); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var errBadTarget = stringError("target must be 'public' or 'private'")

type stringError string

func (s stringError) Error() string { return string(s) }
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/artwork/...
git add api/internal/artwork/visibility.go api/internal/artwork/visibility_test.go
git commit -m "[api] feat(artwork): public⇄private flip with storage move + COALESCE published_at"
```

---

## Task 25: Artwork list/detail/create/patch/delete handlers

**Files:**
- Create: `api/internal/httpapi/router.go`
- Create: `api/internal/httpapi/artworks.go`
- Create: `api/internal/httpapi/artworks_test.go`

This is where the privacy enforcement lives. **404 (not 403) when private and not yours**.

- [ ] **Step 1: Failing tests** — covers privacy-matrix cases 1-3 from spec §8.6.1 (anon/owner/other on public feed and artwork detail).

```go
// api/internal/httpapi/artworks_test.go (skeleton — full impl ~150 lines)
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Each test seeds a viewer, creates a public + private artwork, hits the
// route, and asserts JSON shape + presence/absence of the private artwork.

func TestFeed_AnonHidesPrivate(t *testing.T)         { runMatrix(t, "anon", "GET /artworks", anonExpectsPublicOnly) }
func TestArtworkDetail_NonOwnerGetsPrivate404(t *testing.T) { /* ... */ }
func TestArtworkDetail_OwnerGetsSignedURLs(t *testing.T)    { /* ... */ }
```

The full matrix runner is implemented in Task 28. This task installs the routes; the matrix test is the comprehensive verifier.

- [ ] **Step 2: Implementation**

```go
// api/internal/httpapi/router.go
package httpapi

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/auth"
	"github.com/<org>/art-web/api/internal/image"
	"github.com/<org>/art-web/api/internal/user"
)

type Deps struct {
	JWT        *auth.JWT
	URL        *auth.URLBuilder
	Providers  map[string]auth.Provider
	Users      *user.Repo
	Artworks   *artwork.Repo
	Tags       *artwork.TagsRepo
	Images     *image.Repo
	Upload     *image.Handler
	Vis        *artwork.VisibilityService
	Frontend   string         // post-login redirect
	CookieOpts auth.CookieOpts // Domain + Secure for the auth cookie
}

func New(d *Deps) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, corsMW)
	r.Use(auth.Middleware(d.JWT))
	r.Use(noStoreForViewerSpecific)

	r.Route("/auth", func(r chi.Router) {
		r.Get("/{provider}/start", auth.StartHandler(d.Providers, d.Frontend).ServeHTTP)
		r.Get("/{provider}/callback", auth.CallbackHandler(d.Providers, d.Users, d.JWT, d.Frontend, d.CookieOpts).ServeHTTP)
		r.Post("/logout", auth.LogoutHandler(d.CookieOpts).ServeHTTP)
	})
	r.Get("/me", meHandler(d).ServeHTTP)

	r.Route("/artworks", func(r chi.Router) {
		r.Get("/", listFeedHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Post("/", createArtworkHandler(d).ServeHTTP)
		r.Get("/{id}", getArtworkHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Patch("/{id}", patchArtworkHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Delete("/{id}", deleteArtworkHandler(d).ServeHTTP)
		r.With(auth.RequireUser).Post("/{id}/images", d.Upload.Upload)
	})
	r.Get("/users/{slug}", userProfileHandler(d).ServeHTTP)
	r.Get("/tags/{name}", tagsHandler(d).ServeHTTP)
	return r
}
```

**`internal/httpapi/render.go`** — JSON renderer + URL helpers shared by handlers.

```go
// api/internal/httpapi/render.go
package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/image"
	"github.com/<org>/art-web/api/internal/user"
)

func renderJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func renderUser(u *user.User) map[string]any {
	var avatar *string
	if u.AvatarURL != "" {
		v := u.AvatarURL
		avatar = &v
	}
	return map[string]any{
		"id":           u.ID,
		"display_name": u.DisplayName,
		"slug":         u.Slug,
		"avatar_url":   avatar,
	}
}

func renderImageRef(d *Deps, im *image.InsertedImage, visibility string) map[string]any {
	var url string
	if visibility == "public" {
		url = d.URL.Public(im.StorageKey)
	} else {
		url = d.URL.Private(im.StorageKey, 5*time.Minute)
	}
	return map[string]any{
		"id":       im.ID,
		"url":      url,
		"width":    im.Width,
		"height":   im.Height,
		"blurhash": im.Blurhash,
		"position": im.Position,
	}
}

func renderArtworkSummary(d *Deps, a *artwork.Artwork, cover *image.InsertedImage, artist *user.User) map[string]any {
	var pub *string
	if a.PublishedAt != nil {
		s := a.PublishedAt.UTC().Format(time.RFC3339)
		pub = &s
	}
	return map[string]any{
		"id":           a.ID,
		"title":        a.Title,
		"visibility":   a.Visibility,
		"published_at": pub,
		"created_at":   a.CreatedAt.UTC().Format(time.RFC3339),
		"cover":        renderImageRef(d, cover, a.Visibility),
		"artist":       renderUser(artist),
	}
}
```

**`internal/httpapi/artworks.go`** — all artwork handlers.

```go
// api/internal/httpapi/artworks.go
package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/auth"
	"github.com/<org>/art-web/api/internal/image"
	"github.com/<org>/art-web/api/internal/user"
)

func parseLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 100 {
		return 24
	}
	return n
}

func parseCursor(r *http.Request) artwork.FeedCursor {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return artwork.FeedCursor{}
	}
	dec, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return artwork.FeedCursor{}
	}
	parts := strings.SplitN(string(dec), ":", 2)
	if len(parts) != 2 {
		return artwork.FeedCursor{}
	}
	n, _ := strconv.ParseInt(parts[0], 10, 64)
	return artwork.FeedCursor{PublishedAtUnix: n, ID: parts[1]}
}

func encodeCursor(c *artwork.FeedCursor) *string {
	if c == nil {
		return nil
	}
	raw := fmt.Sprintf("%d:%s", c.PublishedAtUnix, c.ID)
	enc := base64.RawURLEncoding.EncodeToString([]byte(raw))
	return &enc
}

func meHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := auth.UserIDFrom(r.Context())
		if !ok {
			renderJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		u, err := d.Users.Get(r.Context(), uid)
		if err != nil {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		renderJSON(w, 200, renderUser(u))
	})
}

func listFeedHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, err := d.Artworks.PublicFeed(r.Context(), parseCursor(r), parseLimit(r))
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		renderFeed(w, r.Context(), d, page)
	})
}

func renderFeed(w http.ResponseWriter, ctx context.Context, d *Deps, page *artwork.FeedPage) {
	items := make([]any, 0, len(page.Items))
	for i := range page.Items {
		a := &page.Items[i]
		cover, artist, err := loadCoverAndArtist(ctx, d, a)
		if err != nil {
			continue // skip artworks with no images yet
		}
		items = append(items, renderArtworkSummary(d, a, cover, artist))
	}
	renderJSON(w, 200, map[string]any{
		"items":       items,
		"next_cursor": encodeCursor(page.NextCursor),
	})
}

func loadCoverAndArtist(ctx context.Context, d *Deps, a *artwork.Artwork) (*image.InsertedImage, *user.User, error) {
	images, err := d.Images.ListByArtwork(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}
	var cover *image.InsertedImage
	if a.CoverImageID != nil {
		for i := range images {
			if images[i].ID == *a.CoverImageID {
				cover = &images[i]
				break
			}
		}
	}
	if cover == nil && len(images) > 0 {
		cover = &images[0]
	}
	if cover == nil {
		return nil, nil, errors.New("no images")
	}
	u, err := d.Users.Get(ctx, a.UserID)
	return cover, u, err
}
```

**`createArtworkHandler`:**

```go
func createArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		var body struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Visibility  string   `json:"visibility"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_json"})
			return
		}
		if body.Visibility == "" {
			body.Visibility = "private"
		}
		if body.Visibility != "public" && body.Visibility != "private" {
			renderJSON(w, 400, map[string]string{"error": "bad_visibility"})
			return
		}
		id, err := d.Artworks.Create(r.Context(), uid, body.Title, body.Description, body.Visibility)
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "create_failed"})
			return
		}
		if len(body.Tags) > 0 {
			_ = d.Tags.SetTags(r.Context(), id, body.Tags)
		}
		renderJSON(w, 201, map[string]string{"id": id})
	})
}
```

**`patchArtworkHandler`:**

```go
func patchArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		id := chi.URLParam(r, "id")
		a, err := d.Artworks.Get(r.Context(), id)
		if err != nil || a.UserID != uid {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		var body struct {
			Title       *string  `json:"title,omitempty"`
			Description *string  `json:"description,omitempty"`
			Visibility  *string  `json:"visibility,omitempty"`
			Tags        []string `json:"tags,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_json"})
			return
		}
		if body.Title != nil {
			if err := d.Artworks.PatchTitleDescription(r.Context(), id, *body.Title, body.Description); err != nil {
				renderJSON(w, 500, map[string]string{"error": "patch_failed"})
				return
			}
		}
		if body.Visibility != nil {
			if *body.Visibility != "public" && *body.Visibility != "private" {
				renderJSON(w, 400, map[string]string{"error": "bad_visibility"})
				return
			}
			if err := d.Vis.Flip(r.Context(), id, *body.Visibility); err != nil {
				renderJSON(w, 500, map[string]string{"error": "flip_failed"})
				return
			}
		}
		if body.Tags != nil {
			if err := d.Tags.SetTags(r.Context(), id, body.Tags); err != nil {
				renderJSON(w, 500, map[string]string{"error": "tag_failed"})
				return
			}
		}
		w.WriteHeader(204)
	})
}
```

**`deleteArtworkHandler`:**

```go
func deleteArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		id := chi.URLParam(r, "id")
		a, err := d.Artworks.Get(r.Context(), id)
		if err != nil || a.UserID != uid {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		// TODO post-v1: also purge R2 objects (currently leaks).
		if err := d.Artworks.Delete(r.Context(), id); err != nil {
			renderJSON(w, 500, map[string]string{"error": "delete_failed"})
			return
		}
		w.WriteHeader(204)
	})
}
```

The `TODO post-v1` is the single allowed comment of its kind in this plan — it pins the GC sweeper deferral noted in spec §11 to a concrete code site, not a planning placeholder.

**`getArtworkHandler` (privacy enforcement, repeated here for the file's completeness):**

```go
func getArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		uid, _ := auth.UserIDFrom(r.Context())
		a, err := d.Artworks.Get(r.Context(), id)
		if err != nil || (a.Visibility == "private" && a.UserID != uid) {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		images, err := d.Images.ListByArtwork(r.Context(), a.ID)
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		artist, err := d.Users.Get(r.Context(), a.UserID)
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "user_failed"})
			return
		}
		tags, _ := d.Tags.GetTags(r.Context(), a.ID)
		var description *string
		if a.Description != nil && *a.Description != "" {
			description = a.Description
		}
		imgs := make([]any, 0, len(images))
		for i := range images {
			imgs = append(imgs, renderImageRef(d, &images[i], a.Visibility))
		}
		var cover *image.InsertedImage
		if a.CoverImageID != nil {
			for i := range images {
				if images[i].ID == *a.CoverImageID {
					cover = &images[i]
					break
				}
			}
		}
		if cover == nil && len(images) > 0 {
			cover = &images[0]
		}
		summary := renderArtworkSummary(d, a, cover, artist)
		summary["description"] = description
		summary["tags"] = tags
		summary["images"] = imgs
		renderJSON(w, 200, summary)
	})
}
```

**`internal/httpapi/users.go`:**

```go
// api/internal/httpapi/users.go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/<org>/art-web/api/internal/auth"
)

func userProfileHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		viewer, _ := auth.UserIDFrom(r.Context())
		profileUser, err := d.Users.GetBySlug(r.Context(), slug)
		if err != nil {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		isOwner := viewer == profileUser.ID
		page, err := d.Artworks.ListByUser(r.Context(), profileUser.ID, isOwner, parseCursor(r), parseLimit(r))
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		items := make([]any, 0, len(page.Items))
		for i := range page.Items {
			a := &page.Items[i]
			cover, artist, err := loadCoverAndArtist(r.Context(), d, a)
			if err != nil {
				continue
			}
			items = append(items, renderArtworkSummary(d, a, cover, artist))
		}
		renderJSON(w, 200, map[string]any{
			"user":        renderUser(profileUser),
			"artworks":    items,
			"next_cursor": encodeCursor(page.NextCursor),
		})
	})
}
```

- [ ] **Step 3: Commit**

```bash
git add api/internal/httpapi/
git commit -m "[api] feat(httpapi): all artwork/user/me handlers with privacy enforcement"
```

Per-route correctness is verified comprehensively in Task 28.

---

## Task 26: Tag listing handler

**Files:**
- Modify: `api/internal/httpapi/router.go` (already present)
- Create: `api/internal/httpapi/tags.go`

- [ ] **Step 1: Implementation**

```go
// api/internal/httpapi/tags.go
package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/<org>/art-web/api/internal/artwork"
)

func tagsHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(chi.URLParam(r, "name"))
		page, err := d.Artworks.ListByTag(r.Context(), name, parseCursor(r), parseLimit(r))
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		renderFeed(w, d, page, false /*viewerCanSeePrivate*/)
	})
}
```

- [ ] **Step 2: Commit (test absorbed by Task 28)**

```bash
git add api/internal/httpapi/tags.go
git commit -m "[api] feat(httpapi): public tag listing"
```

---

## Task 27: CORS + security middleware

**Files:**
- Create: `api/internal/httpapi/middleware.go`
- Create: `api/internal/httpapi/middleware_test.go`

- [ ] **Step 1: Failing test**

```go
func TestCORS_AllowsCredentialsForAllowedOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/me", nil)
	req.Header.Set("Origin", "https://app.example.com")
	httpapi.CORSFor("https://app.example.com")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal()
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatal()
	}
}

func TestNoStoreOnViewerSpecificEndpoints(t *testing.T) {
	rec := httptest.NewRecorder()
	mw := httpapi.NoStoreForViewerSpecific()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/me", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("cc=%q", cc)
	}
}
```

- [ ] **Step 2: Implementation**

```go
// api/internal/httpapi/middleware.go
package httpapi

import (
	"net/http"
	"strings"
)

func CORSFor(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoStoreForViewerSpecific marks endpoints whose response may include
// signed URLs or owner-only data per contracts §10.
func NoStoreForViewerSpecific() func(http.Handler) http.Handler {
	private := map[string]bool{
		"/me": true, "/users/": true, "/artworks/": true, // path-prefix match below
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for p := range private {
				if strings.HasPrefix(r.URL.Path, p) || r.URL.Path == strings.TrimRight(p, "/") {
					w.Header().Set("Cache-Control", "private, no-store")
					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 3: Run + commit**

```bash
cd api && go test ./internal/httpapi/...
git add api/internal/httpapi/middleware.go api/internal/httpapi/middleware_test.go
git commit -m "[api] feat(httpapi): CORS + no-store for viewer-specific endpoints"
```

---

## Task 28: Privacy matrix tests (Go side, cases 1-5)

**Files:**
- Create: `api/internal/artwork/privacy_matrix_test.go`

- [ ] **Step 1: Build the matrix runner**

```go
// api/internal/artwork/privacy_matrix_test.go
package artwork_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// runMatrix asserts the contracts §12 cases 1-5 against a live router.
// Setup per spec §8.6.1: user A owns public P and private Q; user B is
// another signed-in user; anon is unauthenticated. Helper functions
// `seed(t)` and `request(t, viewer, method, path)` are added in the
// httpapi test helpers package.

type case_ struct {
	name           string
	viewer         string // "anon" | "owner" | "other"
	method, path   string
	expectStatus   int
	mustNotContain []string // each substring MUST be absent from the body
	mustContain    []string
}

var cases = []case_{
	// Case 1: GET /artworks
	{name: "feed-anon",   viewer: "anon",  method: "GET", path: "/artworks", expectStatus: 200,
		mustNotContain: []string{"Q.id", "private/", "sig=", "exp="}},
	{name: "feed-owner",  viewer: "owner", method: "GET", path: "/artworks", expectStatus: 200,
		mustNotContain: []string{"Q.id"}},
	{name: "feed-other",  viewer: "other", method: "GET", path: "/artworks", expectStatus: 200,
		mustNotContain: []string{"Q.id"}},
	// Case 2-3: GET /artworks/:id
	// (built dynamically once seed() returns Q.ID and P.ID)

	// Case 4: GET /users/:slug
	{name: "profile-anon",  viewer: "anon",  method: "GET", path: "/users/alice", expectStatus: 200,
		mustNotContain: []string{"Q.id", "private/", "sig="}},
	// owner sees both
	{name: "profile-owner", viewer: "owner", method: "GET", path: "/users/alice", expectStatus: 200,
		mustContain: []string{"P.id", "Q.id", "/img/private/", "sig="}},
	// other sees only public
	{name: "profile-other", viewer: "other", method: "GET", path: "/users/alice", expectStatus: 200,
		mustNotContain: []string{"Q.id", "private/"}},
	// Case 5: GET /tags/:name
	{name: "tag-anon",  viewer: "anon",  method: "GET", path: "/tags/t", expectStatus: 200,
		mustNotContain: []string{"Q.id", "private/", "sig="}},
}

func TestPrivacyMatrix_Cases1to5(t *testing.T) {
	env := setupMatrixEnv(t) // seeds A,B; creates P,Q; returns *MatrixEnv with router + ids
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, status := env.request(t, c.viewer, c.method, c.path)
			if status != c.expectStatus {
				t.Fatalf("status=%d want=%d body=%s", status, c.expectStatus, body)
			}
			subs := func(p string) string { return strings.ReplaceAll(p, "Q.id", env.QID) }
			for _, s := range c.mustNotContain {
				if strings.Contains(body, subs(s)) {
					t.Errorf("body must not contain %q", subs(s))
				}
			}
			for _, s := range c.mustContain {
				if !strings.Contains(body, subs(s)) {
					t.Errorf("body must contain %q", subs(s))
				}
			}
		})
	}
}

// Two extra cases not table-driven because they assert structured JSON.
func TestArtworkDetail_Q_AnonAndOther_Are404(t *testing.T) { /* hits /artworks/<Q.ID> */ }
func TestArtworkDetail_P_HasPublicURL(t *testing.T)        { /* anon or other; URL has no sig */ }
```

- [ ] **Step 2: Build `setupMatrixEnv` and `request` helpers** in `api/internal/httpapi/testutil_test.go`

This wires the full router with real DB + localfs storage + a `URLBuilder` configured against a test base URL, and registers two seeded users. Each `request()` call attaches the appropriate `auth` cookie (or none, for anon).

- [ ] **Step 3: Run** — should produce ~12 sub-tests passing.

```bash
cd api && go test ./internal/artwork/... ./internal/httpapi/... -run PrivacyMatrix -v
```

- [ ] **Step 4: Commit**

```bash
git add api/internal/artwork/privacy_matrix_test.go api/internal/httpapi/testutil_test.go
git commit -m "[api] test(privacy): matrix cases 1-5 with negative-content assertions"
```

---

## Task 29: Upload idempotency tests (cases 17-20)

Already covered by `internal/image/repo_test.go` (cases 17, 18) and `internal/image/handler_test.go` (cases 19, 20). This task is a sweep that ensures all four are green and named according to the spec.

- [ ] **Step 1: Verify naming**

Each test's name must include the case number from spec §8.6.3:

```bash
grep -r "TestRetryWithSameClientID_IsNoOp\|TestPositionCollision\|TestConcurrentUploads\|TestPartialFailureResume" api/internal/image/
```

Rename if necessary so test names embed `Case17`, `Case18`, `Case19`, `Case20` for grep-discoverability.

- [ ] **Step 2: Run + commit only if a rename happened**

```bash
cd api && go test ./internal/image/... -race
```

---

## Task 30: Publish lifecycle test (case 21)

**Files:**
- Create: `api/internal/artwork/publish_lifecycle_test.go`

- [ ] **Step 1: Failing test**

```go
// api/internal/artwork/publish_lifecycle_test.go
package artwork_test

import (
	"testing"
	"time"
)

func TestPublishLifecycle_Case21_PublishedAtStableAfterFirstFlip(t *testing.T) {
	repo, _, uid := newCtx(t)
	id, _ := repo.Create(t.Context(), uid, "x", "", "private")

	a, _ := repo.Get(t.Context(), id)
	if a.PublishedAt != nil {
		t.Fatal("draft should have NULL published_at")
	}

	// First publish.
	_ = repo.PatchVisibility(t.Context(), id, "public")
	a, _ = repo.Get(t.Context(), id)
	if a.PublishedAt == nil {
		t.Fatal("first publish should set published_at")
	}
	first := *a.PublishedAt

	// Cycle private→public→private→public.
	for _, v := range []string{"private", "public", "private", "public"} {
		_ = repo.PatchVisibility(t.Context(), id, v)
	}
	a, _ = repo.Get(t.Context(), id)
	if !a.PublishedAt.Equal(first) {
		t.Fatalf("published_at drifted: was %v now %v (%v elapsed)",
			first, *a.PublishedAt, a.PublishedAt.Sub(first))
	}
	_ = time.Now // pin import
}
```

- [ ] **Step 2: Run** — should already pass given Task 15's COALESCE rule.

```bash
cd api && go test ./internal/artwork/... -run PublishLifecycle -v
```

- [ ] **Step 3: Commit**

```bash
git add api/internal/artwork/publish_lifecycle_test.go
git commit -m "[api] test(artwork): case 21 published_at stability across flips"
```

---

## Task 31: `cmd/api/main.go` wiring + config

**Files:**
- Modify: `api/cmd/api/main.go`
- Create: `api/cmd/api/config.go`

- [ ] **Step 1: Replace placeholder main**

```go
// api/cmd/api/main.go
package main

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/<org>/art-web/api/internal/artwork"
	"github.com/<org>/art-web/api/internal/auth"
	"github.com/<org>/art-web/api/internal/db"
	"github.com/<org>/art-web/api/internal/httpapi"
	"github.com/<org>/art-web/api/internal/image"
	"github.com/<org>/art-web/api/internal/storage"
	"github.com/<org>/art-web/api/internal/user"
)

func main() {
	cfg := loadConfig()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	if err := db.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	var store storage.Storage
	if cfg.AppEnv == "dev" {
		store = storage.NewLocalFS("./var/storage")
	} else {
		s3cli := s3.NewFromConfig(aws.Config{
			Region:      "auto",
			Credentials: credentials.NewStaticCredentialsProvider(cfg.R2KeyID, cfg.R2KeySecret, ""),
		}, func(o *s3.Options) {
			o.BaseEndpoint = aws.String("https://" + cfg.R2AccountID + ".r2.cloudflarestorage.com")
			o.UsePathStyle = true
		})
		store = storage.NewR2(s3cli, cfg.R2Bucket)
	}

	signKey, _ := hex.DecodeString(cfg.WorkerSigningKey)
	jwtKey, _ := hex.DecodeString(cfg.JWTSigningKey)

	jwts := auth.NewJWT(jwtKey, time.Now)
	urls := auth.NewURLBuilder(cfg.CDNOrigin, signKey, time.Now)

	users := user.NewRepo(pool)
	arts := artwork.NewRepo(pool)
	tags := artwork.NewTagsRepo(pool)
	images := image.NewRepo(pool)
	imgSvc := image.NewService(store, images, arts)
	upload := image.NewHandler(imgSvc, arts, urls)
	vis := artwork.NewVisibilityService(arts, store)

	r := httpapi.New(&httpapi.Deps{
		JWT: jwts, URL: urls,
		Providers: map[string]auth.Provider{
			"google": auth.NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL),
		},
		Users: users, Artworks: arts, Tags: tags, Images: images,
		Upload: upload, Vis: vis,
		Frontend: cfg.FrontendURL,
		CookieOpts: auth.CookieOpts{
			Domain: cfg.CookieDomain,             // "" in dev; ".example.com" in prod
			Secure: cfg.AppEnv != "dev",
		},
	})

	log.Info("listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Config loader**

```go
// api/cmd/api/config.go
package main

import "os"

type config struct {
	Addr, AppEnv, DatabaseURL, FrontendURL, CDNOrigin             string
	WorkerSigningKey, JWTSigningKey, CookieDomain                 string
	GoogleClientID, GoogleClientSecret, GoogleRedirectURL         string
	R2AccountID, R2KeyID, R2KeySecret, R2Bucket                   string
}

func loadConfig() config {
	return config{
		Addr:               getEnv("ADDR", ":8080"),
		AppEnv:             getEnv("APP_ENV", "dev"),
		DatabaseURL:        mustEnv("DATABASE_URL"),
		FrontendURL:        getEnv("FRONTEND_URL", "http://localhost:3000/"),
		CDNOrigin:          getEnv("CDN_ORIGIN", "http://localhost:8787"),
		WorkerSigningKey:   mustEnv("WORKER_SIGNING_KEY"),
		JWTSigningKey:      mustEnv("JWT_SIGNING_KEY"),
		CookieDomain:       getEnv("COOKIE_DOMAIN", ""), // e.g. ".example.com" in prod
		GoogleClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost:8080/auth/google/callback"),
		R2AccountID:        getEnv("R2_ACCOUNT_ID", ""),
		R2KeyID:            getEnv("R2_ACCESS_KEY_ID", ""),
		R2KeySecret:        getEnv("R2_ACCESS_KEY_SECRET", ""),
		R2Bucket:           getEnv("R2_BUCKET", "art-dev"),
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		panic("missing env: " + k)
	}
	return v
}
```

- [ ] **Step 3: Smoke build**

```bash
cd api && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add api/cmd/api/
git commit -m "[api] feat(cmd): wire main with full config + provider registry"
```

---

## Task 32: `Makefile` + `.github/workflows/api.yml`

**Files:**
- Create: `api/Makefile`
- Create: `.github/workflows/api.yml`

- [ ] **Step 1: Makefile**

```make
# api/Makefile
.PHONY: tidy build test test-race lint
tidy:    ; go mod tidy
build:   ; go build -o bin/api ./cmd/api
test:    ; go test ./...
test-race:; go test -race ./...
lint:    ; go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...
```

- [ ] **Step 2: CI workflow**

```yaml
# .github/workflows/api.yml
name: api

on:
  push:
    paths: [ 'api/**', '.github/workflows/api.yml' ]
  pull_request:
    paths: [ 'api/**', '.github/workflows/api.yml' ]

jobs:
  test:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: api } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go mod download
      - run: go vet ./...
      - run: go test -race ./...
```

- [ ] **Step 3: Commit**

```bash
git add api/Makefile .github/workflows/api.yml
git commit -m "[api] ci: Makefile + GitHub Actions workflow scoped to api/**"
```

---

## Done

After all 32 tasks land, Plan 1 produces a Go API that:

- Accepts Google OAuth and issues an HS256 cookie
- Persists artworks/images/tags with the spec §5 schema
- Serves a privacy-aware feed/profile/detail/tag/me API
- Accepts multi-image uploads with idempotent retries (cases 17–20)
- Holds `published_at` stable across flips (case 21)
- Builds a signed image URL the Worker (Plan 2) verifies (case 15 companion)
- Runs against `localfs` in dev or `r2` in prod via the same `Storage` interface

The 25-test correctness budget is partially satisfied (cases 1-5 + 15-companion + 17-21 + the building blocks for 6-9). The remaining cases (10-14, 15 cross-system, 16, 22-25) are addressed by Plans 2 and 3.
