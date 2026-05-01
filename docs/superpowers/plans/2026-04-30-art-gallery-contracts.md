# Art Gallery — Cross-Plan Contracts

**Date:** 2026-04-30
**Status:** Frozen (any change requires updating all three plans)
**Source spec:** [2026-04-30-art-gallery-design.md](../specs/2026-04-30-art-gallery-design.md)

This document defines the interfaces shared between the three implementation plans. Anything in here is fixed: a change requires editing this file *and* every plan that depends on it. Anything not in here is free for each plan to choose locally.

---

## 1. Monorepo Layout

```
art-web/
├── api/                       # Plan 1 — Go API + PostgreSQL
│   ├── cmd/api/main.go
│   ├── internal/{httpapi,auth,user,artwork,image,storage,db}/
│   ├── migrations/
│   ├── go.mod
│   └── go.sum
├── worker/                    # Plan 2 — Cloudflare Worker
│   ├── src/{index.ts,sign.ts,allowlist.ts}
│   ├── test/
│   ├── package.json
│   ├── wrangler.toml
│   └── tsconfig.json
├── web/                       # Plan 3 — Next.js frontend
│   ├── app/
│   ├── components/
│   ├── lib/
│   ├── e2e/
│   ├── package.json
│   ├── next.config.js
│   ├── playwright.config.ts
│   └── vitest.config.ts
├── docs/superpowers/{specs,plans}/
└── .gitignore
```

**Disjoint-subtree rule:** Plans 1/2/3 own `api/`, `worker/`, `web/` respectively and never touch each other's subtrees. The only shared file all three may modify is `.gitignore` and `README.md` at root, and only via additive entries.

## 2. Parallel Execution Model

Three worktrees on three branches, run in parallel:

| Worktree | Branch | Owns | Depends on |
|---|---|---|---|
| `.worktrees/api`     | `feature/api`     | `api/`     | this contracts doc only |
| `.worktrees/worker`  | `feature/worker`  | `worker/`  | this contracts doc only |
| `.worktrees/web`     | `feature/web`     | `web/`     | this contracts doc + a running API + a running Worker for E2E |

**Sequencing rule:** Plans 1 and 2 can start the *moment* this contracts doc is merged to master. Plan 3 can also start in parallel using mocked API/Worker responses for its unit/component tests (Vitest + RTL); Plan 3's full Playwright E2E suite is the single convergence point that requires Plans 1 and 2 to be merged.

**Worktree creation (each is its own command, run from master):**

```bash
git worktree add .worktrees/api    -b feature/api    master
git worktree add .worktrees/worker -b feature/worker master
git worktree add .worktrees/web    -b feature/web    master
```

Three independent agents (or three sequential sessions) can pick a worktree and run their plan to completion without coordination beyond this contracts doc.

## 3. HMAC Signing Scheme (used by Plan 1 to generate, Plan 2 to verify)

**String-to-sign:**

```
v1|<canonicalPath>|<exp>
```

where:

- `<canonicalPath>` always begins with `/private/` (signing is only required for private; public paths are not signed). It is the storage key with a leading slash, **no `/img/` prefix**.
  - Example: storage key `private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0a1b2c3d-4e5f-6789-abcd-ef0123456789.jpg` → canonicalPath `/private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0a1b2c3d-4e5f-6789-abcd-ef0123456789.jpg`.
  - Allowed character set: `[A-Za-z0-9/._-]` only — UUIDs and ASCII extensions never need URL-encoding.
  - Forbidden substrings: `//`, `/../`, `/./`, trailing `/`. Both signer and verifier must reject these.
  - Case-sensitive byte-for-byte comparison.
- `<exp>` is a Unix timestamp in seconds, encoded as a base-10 ASCII string with no leading zeros (e.g. `1745963271`, never `01745963271`).
- The `|` separator is U+007C (single ASCII byte). No whitespace anywhere.

**Algorithm:** HMAC-SHA256.
**Output encoding:** lowercase hexadecimal (64 chars).
**Comparison:** constant-time. `===` / `==` / `bytes.Equal` are forbidden in the verifier; use `crypto/subtle.ConstantTimeCompare` (Go) or `crypto.subtle.timingSafeEqual` (WebCrypto via custom helper) on equal-length buffers.

**Key:** environment variable `WORKER_SIGNING_KEY`, identical bytes loaded into both the Go API and the Cloudflare Worker. Encoded as hex when stored in env (so binary safety is unambiguous). Min 32 bytes (256 bits) decoded.

**TTL:** the API issues `exp = now + 5 minutes`. Five-minute window is the trade-off documented in spec §8.4.

**Public paths are never signed:** `<canonicalPath>` starting with `/public/` carries no `sig` or `exp`. The Worker MUST reject any `/private/` path missing or carrying invalid `sig`/`exp`, and MUST ignore any `sig`/`exp` on a `/public/` path.

## 4. Image URL Shape

Origin URL produced by the API and consumed by `next/image` via the FE custom loader:

```
Public:   https://cdn.example.com/img/<storage_key>
Private:  https://cdn.example.com/img/<storage_key>?sig=<HEX>&exp=<UNIX>
```

The FE loader (`web/lib/cf-loader.ts`) appends transform params per render:

```
&w=<width>&fmt=<format>&q=<quality>
```

Final URL after the loader:

```
https://cdn.example.com/img/<storage_key>?[sig=&exp=&]w=<W>&fmt=<F>&q=<Q>
```

Order of query params is unspecified — both API and Worker must parse them independently of order.

Dev override: when `NEXT_PUBLIC_CDN_BASE` is set (e.g. `http://localhost:8787`), all three components use that origin instead of `https://cdn.example.com`. Path/query shape is identical.

## 5. Storage Key Layout (R2 keys, used in DB column `artwork_images.storage_key`)

```
<visibility>/<artwork_id>/<image_id>.<ext>
```

| Token | Type | Source |
|---|---|---|
| `<visibility>` | literal `public` or `private` | mirrors `artworks.visibility` at upload time |
| `<artwork_id>` | UUID v4 string (lowercased, hyphenated) | `artworks.id` |
| `<image_id>`   | UUID v4 string (lowercased, hyphenated) | `artwork_images.id` (server-generated, distinct from `client_image_id`) |
| `<ext>`        | `jpg` for `image/jpeg`, `png` for `image/png` | derived from `content_type` at upload |

Reordering an artwork's images updates only `artwork_images.position`; the `image_id` and storage key are stable.

A privacy flip moves the object across the `public/` ↔ `private/` prefixes (R2 has no native move; it's a copy + delete) and updates `storage_key` in the same DB transaction.

## 6. Worker Allowlists

| Param | Allowed values | Default if missing |
|---|---|---|
| `w`   | exactly one of `240, 480, 800, 1024, 1600, 2400` | absent (no resize) |
| `fmt` | one of `auto, avif, webp, jpeg` | `auto` |
| `q`   | one of `60, 75, 85, 90` | `85` |

When `fmt=auto`, the Worker negotiates from the request `Accept` header: `image/avif` if accepted, else `image/webp`, else `image/jpeg`.

Anything outside the allowlists is rejected with HTTP 400 and a body `Bad request: <param> not in allowlist`. The `next/image` `deviceSizes` array on the FE side mirrors `w` exactly so generated `srcset` values are always in the allowlist.

## 7. Auth Cookie

| Field | Value |
|---|---|
| Name           | `auth` |
| Value format   | HS256 JWT, claims `{ "sub": "<user_id>", "iat": <unix>, "exp": <unix> }` (`exp` = `iat + 7d`) |
| Signing key    | env `JWT_SIGNING_KEY`, ≥ 32 bytes when hex-decoded |
| `HttpOnly`     | yes |
| `Secure`       | yes (production); allow plain HTTP in dev when `APP_ENV=dev` |
| `SameSite`     | `Lax` |
| `Domain`       | `.example.com` (production); unset in dev |
| `Path`         | `/` |
| `Max-Age`      | `604800` (7 days) |

Set on `/auth/*/callback`. Cleared by `POST /auth/logout` via `Set-Cookie: auth=; Max-Age=0; Path=/; ...same flags...`.

The API trusts the cookie. The Worker does **not** read the cookie — Worker auth is exclusively via signed URL.

## 8. API JSON Shapes (consumed by Plan 3)

All endpoints respond `Content-Type: application/json; charset=utf-8`. Times are RFC 3339 strings with offset (`2026-04-30T14:32:11Z`).

### 8.1 `User`

```ts
type User = {
  id: string;            // UUID
  display_name: string;
  slug: string;          // unique URL-safe handle
  avatar_url: string | null;
};
```

### 8.2 `ImageRef`

```ts
type ImageRef = {
  id: string;            // artwork_images.id
  url: string;           // origin URL per §4 (already signed if private)
  width: number;         // intrinsic px
  height: number;        // intrinsic px
  blurhash: string;      // BlurHash string, ~30 chars
  position: number;      // ordering within the artwork, 0-based
};
```

### 8.3 `ArtworkSummary` (feed/profile/tag listings)

```ts
type ArtworkSummary = {
  id: string;
  title: string;
  visibility: "public" | "private";
  published_at: string | null;          // null until first publish
  created_at: string;
  cover: ImageRef;                      // image where position == artworks.cover_position
  artist: User;
};
```

### 8.4 `ArtworkDetail`

```ts
type ArtworkDetail = ArtworkSummary & {
  description: string | null;
  tags: string[];                       // lowercased tag names
  images: ImageRef[];                   // ordered by position ascending
};
```

### 8.5 `Feed`

```ts
type Feed = {
  items: ArtworkSummary[];
  next_cursor: string | null;           // null when no more pages
};
```

### 8.6 `UserProfile`

```ts
type UserProfile = {
  user: User;
  artworks: ArtworkSummary[];           // visibility-filtered per viewer
  next_cursor: string | null;
};
```

### 8.7 `Cursor`

Opaque base64-url-encoded string. Internally `<published_at_unix_seconds>:<id>`. Never exposed in spec; clients pass it back unchanged.

### 8.8 `ApiError`

```ts
type ApiError = {
  error: string;       // machine code, snake_case, e.g. "not_found", "invalid_position"
  message: string;     // human-readable, never echoes whether a hidden artwork exists
};
```

## 9. HTTP Status Code Conventions

| Code | When |
|---|---|
| 200 | OK |
| 201 | resource created (artwork POST, image POST first time) |
| 204 | DELETE success |
| 400 | malformed request body or unknown query param |
| 401 | missing/invalid auth cookie on a protected route, or missing/invalid signature on a `/private/` Worker URL |
| 403 | **only** for explicit owner actions on a *visible* but not-owned resource where existence is already public; **never** for "private and not yours" — that returns 404 |
| 404 | resource missing OR private and viewer is not the owner (existence-hiding rule, spec §8.5) |
| 409 | display-name slug conflict; OAuth provider/subject already linked to another account; **upload idempotency: same `client_image_id` reused with different bytes (sha256 mismatch)** |
| 412 | upload idempotency: position collision with a different `client_image_id` |
| 415 | request `Content-Type` is not `multipart/form-data` on a multipart endpoint (per-file `content_type` field validation returns 400 instead — that's payload-shape, not HTTP-level) |
| 422 | image larger than 25 MB, decode failure, dimension extraction failure |
| 500 | unexpected server error |

## 10. HTTP Cache-Control Headers

| Surface | Header |
|---|---|
| API JSON for public-only endpoints (`/artworks`, `/tags/:name`) | `Cache-Control: public, max-age=60` (short, edge-cache friendly) |
| API JSON for any endpoint that *could* embed signed URLs (`/me`, `/users/:slug`, `/artworks/:id`) | `Cache-Control: private, no-store` |
| Worker public image responses | `Cache-Control: public, max-age=31536000, immutable` |
| Worker private image responses | `Cache-Control: private, no-store` |
| Next.js public-only pages (`/`, `/tag/[name]`) | default cacheable; default fetch cache |
| Next.js viewer-specific pages (`/u/[slug]`, `/art/[id]`, `/upload`, `/settings`) | `export const dynamic = 'force-dynamic'; export const fetchCache = 'force-no-store';` plus all server fetches use `{ cache: 'no-store' }` |

## 11. Environment Variables (shared keys)

| Variable | Where | Notes |
|---|---|---|
| `WORKER_SIGNING_KEY` | API + Worker | hex-encoded, min 32 bytes decoded; identical bytes |
| `JWT_SIGNING_KEY` | API only | hex-encoded, min 32 bytes decoded |
| `GOOGLE_OAUTH_CLIENT_ID` | API only | |
| `GOOGLE_OAUTH_CLIENT_SECRET` | API only | |
| `R2_ACCOUNT_ID` | API only (Worker uses binding) | |
| `R2_ACCESS_KEY_ID` | API only | for direct R2 PUT |
| `R2_ACCESS_KEY_SECRET` | API only | for direct R2 PUT |
| `R2_BUCKET` | API + Worker | identical bucket name |
| `CDN_ORIGIN` | API only | base URL emitted in image responses; defaults `https://cdn.example.com` |
| `COOKIE_DOMAIN` | API only | e.g. `.example.com` in prod; empty in dev |
| `ALLOWED_ORIGIN` | API only | CORS Access-Control-Allow-Origin; the FE host (e.g. `https://example.com`) |
| `APP_ENV` | API only | `dev` sets `Secure=false` on the auth cookie |
| `NEXT_PUBLIC_API_BASE` | Web only | e.g. `https://api.example.com` |
| `NEXT_PUBLIC_CDN_BASE` | Web only | optional dev override; if unset, the loader uses the host of the URL the API returned |

## 12. Test Distribution Across Plans

The 25 critical-correctness test cases from spec §8.6 are distributed:

| Spec § | Cases | Plan | File |
|---|---|---|---|
| 8.6.1 (privacy matrix — JSON) | 1, 2, 3, 4, 5 | Plan 1 | `api/internal/artwork/privacy_matrix_test.go` |
| 8.6.1 (privacy matrix — Worker) | 10, 11, 12, 13, 14 | Plan 2 | `worker/test/privacy_matrix.spec.ts` |
| 8.6.1 (privacy matrix — SSR HTML) | 6, 7, 8, 9 | Plan 3 | `web/e2e/privacy_html.spec.ts` |
| 8.6.2 (HMAC + cache safety) | 15, 16 | Plan 2 | `worker/test/round_trip.spec.ts`, `worker/test/cache_safety.spec.ts` |
| 8.6.2 (signer correctness, Go side) | (companion of 15) | Plan 1 | `api/internal/auth/signurl_test.go` |
| 8.6.3 (upload idempotency) | 17, 18, 19, 20 | Plan 1 | `api/internal/image/upload_idempotency_test.go` |
| 8.6.4 (publish lifecycle) | 21 | Plan 1 | `api/internal/artwork/publish_lifecycle_test.go` |
| 8.6.5 (UX) | 22, 23, 24, 25 | Plan 3 | `web/e2e/ux.spec.ts` |

Case 25 (flip-private + incognito) is the final integration test that asserts all three subsystems agree. It runs only against a docker-compose'd full stack.

## 13. CI Job Topology

Each plan contributes its own GitHub Actions workflow file under `.github/workflows/`:

- `api.yml` — runs on `api/**` changes; spawns Postgres + MinIO testcontainers; `go test ./...` (Plan 1)
- `worker.yml` — runs on `worker/**` changes; `vitest` against `@cloudflare/vitest-pool-workers` (Plan 2)
- `web.yml` — runs on `web/**` changes; Vitest unit + RTL component tests (Plan 3)
- `e2e.yml` — runs on any-change to `api/**`, `worker/**`, `web/**`; brings up docker-compose stack and runs Playwright (Plan 3 owns this file)

Each plan's workflow file is added in that plan's tasks and modifies only its own file.

## 14. Change Protocol

If implementation reveals a contract here is wrong:

1. Stop work in the affected worktree.
2. Open a PR that edits this contracts doc + every plan task that depends on the changed contract.
3. Land that PR before resuming.

Drift between this doc and a plan is a bug. Both are wrong simultaneously, never one or the other.
