# art-web

Image-sharing app split into three deployable units:

| Service | Stack | Port |
|---|---|---|
| `api/` | Go 1.25, chi, pgx, S3 SDK | 8080 |
| `worker/` | Cloudflare Worker (TS), HMAC-signed image transforms | 8787 |
| `web/` | Next.js 14 App Router, RSC + ISR | 3000 |

Storage in dev is **MinIO** (S3-compatible) standing in for Cloudflare R2. Database is **Postgres 16**.

```
┌──────┐    ┌──────┐    ┌──────────┐
│ web  │───▶│ api  │───▶│ postgres │
│ :3000│    │ :8080│    └──────────┘
│      │           │    ┌──────────┐
│      │───────────┴───▶│  minio   │
│      │    ┌──────┐    │  :9000   │
│      │───▶│worker│───▶│ (R2 shim)│
└──────┘    │ :8787│    └──────────┘
            └──────┘
```

---

## Quick start (Docker Compose)

Brings the entire stack up. Postgres → MinIO → bucket-init → API → Worker → Web, gated by health checks:

```bash
cd web
docker compose -f docker-compose.e2e.yml up --build
```

Open <http://localhost:3000>. The page is empty until you seed.

### Seed the database

The API only exposes `/dev/seed` when `APP_ENV=test` (already set by compose):

```bash
# Two artworks (one public, one private) for two users:
curl -X POST http://localhost:8080/dev/seed

# Plus 50 bulk public artworks for an obviously-populated feed:
curl -X POST 'http://localhost:8080/dev/seed?many=50'
```

The home page caches the SSR feed for 60 s (`revalidate = 60`). After seeding, either wait a minute or restart the web container to flush the ISR cache:

```bash
docker compose -f docker-compose.e2e.yml exec web sh -c 'rm -rf .next/cache' \
  && docker compose -f docker-compose.e2e.yml restart web
```

### Sign in as a seeded user

`POST /dev/seed` returns ready-to-paste auth cookies:

```bash
curl -sX POST http://localhost:8080/dev/seed | jq -r .aliceCookie
# auth=eyJhbGciOiJIUzI1NiIs...
```

In the browser DevTools → **Application → Cookies → http://localhost:3000** add a cookie:
- Name: `auth`
- Value: the JWT after `auth=`
- Path: `/`

Reload — the nav switches to authenticated mode.

### Tear down

```bash
docker compose -f docker-compose.e2e.yml down -v   # -v drops postgres + minio volumes
```

---

## Manual dev mode (hot reload)

Run each service natively for fast feedback. Start infra with compose, then native processes for the three apps.

**Prereqs:** Docker, Go 1.25, [Bun](https://bun.com) 1.x.

```bash
# 1. Infra only
cd web
docker compose -f docker-compose.e2e.yml up postgres minio minio-init
```

```bash
# 2. API
cd api
export APP_ENV=test
export DATABASE_URL='postgres://art:art@localhost:5432/artweb?sslmode=disable'
export JWT_SIGNING_KEY='3031323334353637383961626364656630313233343536373839616263646566'
export WORKER_SIGNING_KEY='3031323334353637383961626364656630313233343536373839616263646566'
export S3_ENDPOINT='http://localhost:9000'
export R2_ACCESS_KEY_ID=minioadmin
export R2_ACCESS_KEY_SECRET=minioadmin
export R2_BUCKET=art-dev
export CDN_ORIGIN='http://localhost:8787'
go run ./cmd/api
```

```bash
# 3. Worker
cd worker
bun install
bun run dev          # wrangler dev → :8787
```

```bash
# 4. Web
cd web
bun install
bun run dev          # next dev → :3000
```

> **Note**: the dev keys above are 32-byte fixtures from the compose file — fine for local, never use in production.

---

## Tests

Each subtree owns its own suite. Targeted commands keep CI fast and avoid running cross-language tests you didn't change.

```bash
# API (Go) — uses testcontainers, needs Docker running
cd api && go test ./...

# Worker (TS) — vitest-pool-workers
cd worker && bun run test

# Web (TS) — vitest unit
cd web && bun run test

# Web E2E — Playwright against the full compose stack
cd web && bun run test:e2e
```

---

## Project layout

```
api/        Go service — auth, artworks, uploads, dev seeding
worker/     Cloudflare Worker — public + HMAC-signed image paths
web/        Next.js 14 App Router — SSR home, profile, upload, settings
docs/       Specs, plans, contracts
go.work     Go workspace pointing at api/ (gitignored — machine-local)
```

---

## Image storage

Images live in **MinIO** at the `art-dev` bucket (R2 stand-in for local dev). Three places hold image-related state, and only one of them holds the bytes:

| Where | What | Authoritative for |
|---|---|---|
| **MinIO** `art-dev` bucket | Raw PNG/JPEG bytes | the actual image file |
| **Postgres** `artwork_images` | `storage_key` + metadata (width, height, mime, blurhash, sha256) | which key belongs to which artwork |
| **Worker** (`web-worker-1`) | Stateless | reads from MinIO via S3 client, transforms on the fly |

### How a storage_key flows through the system

```
┌─────────────────┐         ┌──────────────────┐
│ Postgres        │         │ MinIO            │
│ artwork_images  │         │ Bucket: art-dev  │
│   storage_key:  │ ──────> │   public/<aid>/  │
│   public/aid/.. │         │     <iid>.png    │
└────────┬────────┘         └────────┬─────────┘
         │                           │
         │ API serves                │ Worker reads
         │ cover.url =               │ via S3 client
         │   <CDN_BASE>/img/         │ (e2e-server.ts)
         │   <storage_key>           │
         ▼                           ▼
┌─────────────────────────────────────────────────┐
│ Browser GET                                     │
│   http://localhost:8787/img/public/<aid>/<iid>  │
│   ?w=480&fmt=auto&q=85                          │
└─────────────────────────────────────────────────┘
```

Key layout: `{visibility}/{artworkID}/{imageID}.png` where `visibility` is `public` or `private`. Private keys require an HMAC-signed worker URL (`?sig=…&exp=…`); public keys are reachable by anyone who has the URL.

### Inspecting storage

**MinIO console (web UI)** — the console runs alongside the S3 API on port `9001`. To browse images interactively:

1. Open <http://localhost:9001>
2. Sign in with username `minioadmin` and password `minioadmin`
3. In the left nav, click **Object Browser**
4. Click the `art-dev` bucket
5. Drill into `public/` or `private/` → `<artworkID>/` → click any `.png` to **preview**, **download**, **share**, or **delete**

Use the **Upload** button at the top right to add bytes at any path — handy for swapping in a real image without touching the DB.

**`mc` inside the container:**

```bash
docker exec web-minio-1 mc alias set local http://localhost:9000 minioadmin minioadmin
docker exec web-minio-1 mc ls --recursive local/art-dev/
docker exec web-minio-1 mc stat   local/art-dev/public/<aid>/<iid>.png
docker exec web-minio-1 mc cp     local/art-dev/public/<aid>/<iid>.png /tmp/out.png
docker cp web-minio-1:/tmp/out.png ./out.png
```

**`mc` from the host** (`brew install minio/stable/mc`):

```bash
mc alias set localdev http://localhost:9000 minioadmin minioadmin
mc ls --recursive localdev/art-dev/
```

**S3 API directly** with the AWS CLI:

```bash
AWS_ACCESS_KEY_ID=minioadmin AWS_SECRET_ACCESS_KEY=minioadmin \
  aws --endpoint-url http://localhost:9000 s3 ls s3://art-dev/public/
```

**Postgres → key lookup** (which S3 keys back which artwork):

```bash
docker exec web-postgres-1 psql -U art -d artweb -c \
  "SELECT a.title, ai.storage_key, ai.width, ai.height
     FROM artworks a JOIN artwork_images ai ON ai.artwork_id = a.id
     ORDER BY a.created_at DESC LIMIT 5;"
```

### Replacing a placeholder with a real image

The DB columns `width`, `height`, `byte_size`, `source_sha256` are advisory — the worker doesn't validate them on read. Drop new bytes at the same `storage_key` and the next request gets the new image:

```bash
mc cp ./myart.png localdev/art-dev/public/<artworkID>/<imageID>.png
```

> **Heads up**: the worker sets `Cache-Control: public, max-age=31536000, immutable` on public images. After replacing bytes, browsers that already loaded the old URL won't re-fetch — bump the URL with a cache-buster or clear site data when iterating.

### Volume lifecycle

`/data` inside the MinIO container is an **anonymous Docker volume**. It survives `docker compose stop` / `up -d` / container recreates, but is **destroyed by `docker compose down -v`**. After a `down -v` you must reseed (`POST /dev/seed?many=N`) to repopulate the bucket.

---

## Troubleshooting

### `NoSuchBucket: art-dev`
The MinIO bucket disappears when you `down -v`. The `minio-init` service in compose creates it before the API starts; if you skipped it (e.g. running API natively against a fresh MinIO), create it manually:

```bash
docker run --rm --network web_default --entrypoint sh minio/mc -c \
  "mc alias set m http://minio:9000 minioadmin minioadmin && mc mb -p m/art-dev"
```

### Home page is blank after seeding
ISR cache. See "Seed the database" above for the cache-flush command. Or just wait 60 s.

### Docker build fails with `apk add` SSL `certificate verify failed`
Host machine is doing TLS interception (corporate proxy, VPN). The api Dockerfile already switches apk repos to HTTP and copies a CA bundle from the build stage — if you see this in a different image, apply the same pattern:

```dockerfile
RUN sed -i 's|https://|http://|g' /etc/apk/repositories && apk add --no-cache <pkg>
```

### `go: module … requires go >= 1.25.0`
You're on Go 1.24. Bump your local Go to 1.25, or rebuild via Docker which pins the toolchain.

### `cannot find module 'pngjs/browser'` during `bun run build`
`web/types/pngjs-browser.d.ts` declares the missing types. If you delete or relocate it, restore it — the package itself ships no types.

---

## Endpoints reference

| Endpoint | Method | Auth | Purpose |
|---|---|---|---|
| `/healthz` | GET | — | API liveness (also on worker, web) |
| `/artworks?limit=N&cursor=…` | GET | — | Public feed (cursor-paginated) |
| `/artworks/{id}` | GET | optional | Single artwork (privacy-aware) |
| `/artworks` | POST | required | Create artwork |
| `/artworks/{id}/images` | POST | owner | Upload image bytes |
| `/auth/google/start` | GET | — | OAuth entry |
| `/me` | GET | — | Current user (or 401) |
| `/dev/seed` | POST | — (test only) | Seed users + artworks |
| `/img/public/{art}/{img}.png` | GET | — | Worker: public image |
| `/img/private/{art}/{img}.png?sig=…&exp=…` | GET | HMAC | Worker: private image |
