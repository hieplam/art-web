# Art Gallery Web App — Design Spec

**Date:** 2026-04-30 (last revised 2026-05-01)
**Status:** Approved
**Owner:** Todd Lam

**Revision history:**

- 2026-05-01 — Schema amendments from plan-1 code review:
  - `artworks.cover_image_id uuid` (with FK to `artwork_images.id`) → `artworks.cover_position int NOT NULL DEFAULT 0`. Original FK only enforced existence, not same-artwork; a buggy or malicious update could point an artwork's cover at an image owned by *another* artwork. The new representation makes the bug structurally impossible: cover is the row where `(artwork_id, position) = (artwork_id, cover_position)`.
  - Added `artwork_images.source_sha256 text NOT NULL`. Idempotency keyed only on `client_image_id` could silently accept a buggy retry that reused the key with different bytes — the server would return the original row and the client would be told "OK, already uploaded" for the wrong file. Storing a SHA-256 fingerprint lets the upload pipeline distinguish "true retry" from "key reuse with different bytes" (the latter returns 409).
- 2026-05-01 (later) — Pagination + privacy-flip amendments from plan-1 second review:
  - Cursor wire format pinned to nanosecond precision (RFC3339Nano + UUID, see contracts §8.7). The earlier draft truncated `published_at` to integer epoch seconds, which silently dropped items whose timestamps shared a second because the lexicographic tuple fall-through was on a random UUID. No item is duplicated *or* skipped now.
  - §6.6 privacy flip extended to cover the `private → public` partial-failure direction. Earlier text only described the safe `public → private` direction; without rollback, a partial private→public flip would leave already-moved bytes accessible at their unsigned `/public/` path despite the artwork still being marked private in DB. Implementation now rolls back already-moved objects on any error before returning.

## 1. Overview

A web platform where artists upload their artwork (multi-image works) and choose to publish it as **public** (visible to anyone, including anonymous visitors) or **private** (visible only to the owner). Anonymous visitors can browse a homepage feed of all public artworks, click any artwork to view full-resolution images with the artist's name and publish date, and visit per-artist profile pages.

The product targets a smooth, low-latency browsing experience comparable to ArtStation/Pinterest despite delivering many high-resolution images.

## 2. Scope (v1)

**In scope:**

- Sign in via Google OAuth2
- Upload an artwork composed of one or more JPG/PNG images (max 25 MB per image)
- Set artwork visibility (public or private), edit title, description, and tags
- Public homepage feed — masonry layout, reverse-chronological, paginated (infinite scroll)
- Per-artist profile page (`/u/<slug>`) listing the artist's public artworks (and private ones, if the viewer is the owner)
- Artwork detail page (`/art/<id>`) with full-resolution images, artist name, publish date, tags
- Tag pages (`/tag/<name>`) listing public artworks tagged with that name
- Profile editing (display name, slug, avatar)

**Explicitly out of scope** (deferred to a later release):

- Likes, comments, follows
- Full-text search
- NSFW gating, content moderation, reporting
- Collections, portfolios beyond the profile timeline
- Drag-to-reorder UI within an artwork
- Image variants pre-generated at upload (we use on-demand CDN transforms)
- Lightbox / intercepting routes (dedicated detail page only)
- Facebook / Instagram OAuth (Google only; auth layer is provider-pluggable)
- GIF / WebP source formats
- Notifications, email digests
- Mobile apps
- Admin tools
- Internationalization

## 3. Stack & Constraints

| Concern | Choice |
|---|---|
| Backend language | Go |
| Backend HTTP router | `chi` (lightweight, idiomatic) |
| Database | PostgreSQL |
| Migrations | `golang-migrate` |
| Object storage | Cloudflare R2 (prod), local filesystem (dev), behind a `Storage` interface |
| Image transform | Cloudflare Workers Images binding (`env.IMAGES`) — applied inside the Worker, not via `/cdn-cgi/image/` |
| Access control on storage | Cloudflare Worker is the single chokepoint at `cdn.example.com/img/...`; reads R2 via binding, validates signed URLs for private content |
| Frontend framework | Next.js (App Router, React Server Components) |
| Image rendering | `next/image` with custom Cloudflare loader |
| Auth | OAuth2 → Google; HTTP-only signed cookie issued by the Go API |
| Testing (Go) | `go test` + testcontainers (Postgres, MinIO) |
| Testing (Next) | Vitest + React Testing Library + Playwright |

## 4. Architecture

```
┌─────────────────────┐         ┌──────────────────────┐
│  Next.js            │  HTTPS  │   Go API             │
│  example.com        ├────────▶│   api.example.com    │
│  - SSR + RSC        │  JSON   │   - REST endpoints   │
│  - next/image       │         │   - JWT in cookie    │
└─────────┬───────────┘         └──────┬───────────────┘
          │                            │
          │ <img src=cdn/img/...?w=800> │ SQL
          ▼                            ▼
┌──────────────────────────────┐  ┌──────────────────────┐
│  Cloudflare Worker           │  │   PostgreSQL         │
│  cdn.example.com/img/...     │  │   (managed or self)  │
│                              │  └──────────────────────┘
│  1. Validate sig (private)   │
│  2. R2.get(key)              │ ◀── R2 binding
│  3. IMAGES.transform({w,...})│ ◀── Cloudflare Images binding
│  4. Set Cache-Control        │
│  5. Return                   │
└─────────┬────────────────────┘
          │ R2 binding
          ▼
┌─────────────────────┐
│  Cloudflare R2      │
│  (private bucket)   │
│  public/...         │
│  private/...        │
└─────────────────────┘
```

**Data flows:**

- **Browser → Next.js**: HTML page requests, all client-side `fetch` for incremental data.
- **Next.js → Go API**: server-side fetch during SSR, forwarding the request's cookies to authenticate as the user.
- **Browser → Worker**: direct `<img>` requests to `cdn.example.com/img/...`, never via Next or Go. This is the principal "no lag" lever.
- **Worker → R2**: read source bytes via R2 binding (no public R2 URL exists; the binding is the only access path).
- **Worker → IMAGES**: pipe source bytes through the `env.IMAGES` binding to apply transform (resize, format conversion).
- **Worker decisions**: `/img/public/*` passes auth; `/img/private/*` requires a valid HMAC-signed query before reading R2.

The API never proxies image bytes. The Worker is the single chokepoint that owns auth, transform, and cache headers — no other service can serve image bytes.

## 5. Data Model (PostgreSQL)

```sql
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
  cover_position int NOT NULL DEFAULT 0,                      -- which artwork_images.position is the cover
  created_at     timestamptz NOT NULL DEFAULT now(),
  published_at   timestamptz                                 -- NULL until first publish
);

-- Public feed (anonymous browse, others viewing a profile)
CREATE INDEX artworks_public_feed_idx ON artworks (published_at DESC)
  WHERE visibility = 'public' AND published_at IS NOT NULL;

-- Per-user view (owner sees their own, including drafts)
CREATE INDEX artworks_by_user_idx ON artworks (user_id, created_at DESC);

CREATE TABLE artwork_images (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),  -- server-generated; used in storage path
  artwork_id      uuid NOT NULL REFERENCES artworks(id) ON DELETE CASCADE,
  client_image_id uuid NOT NULL,                               -- client-generated; idempotency key
  storage_key     text NOT NULL,
  source_sha256   text NOT NULL,                               -- hex SHA-256 of original bytes; fingerprint
  width           int NOT NULL,
  height          int NOT NULL,
  byte_size       int NOT NULL,
  content_type    text NOT NULL,
  position        int NOT NULL,                                -- client-supplied order within artwork
  blurhash        text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (artwork_id, client_image_id),                        -- idempotency: retry returns same row
  UNIQUE (artwork_id, position)                                -- ordering: positions exclusive within artwork
);

CREATE TABLE tags (
  id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text UNIQUE NOT NULL    -- lowercased, trimmed at insert
);

CREATE TABLE artwork_tags (
  artwork_id uuid NOT NULL REFERENCES artworks(id) ON DELETE CASCADE,
  tag_id     uuid NOT NULL REFERENCES tags(id),
  PRIMARY KEY (artwork_id, tag_id)
);
CREATE INDEX artwork_tags_by_tag_idx ON artwork_tags (tag_id);
```

**Notes:**

- All primary keys are `uuid` generated by `gen_random_uuid()` (Postgres 13+ built-in). UUIDs appear directly in URLs (e.g., `/art/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f`); they're already unguessable, so we don't need a separate URL slug.
- Random UUID v4 is mildly suboptimal for B-tree index locality on insert-heavy tables. At v1 scale (a few uploads per minute at most) this is invisible; if writes ever dominate, switching to UUID v7 (time-ordered) is a one-column-default change.
- `published_at` is **nullable**: `NULL` while the artwork is private/draft, populated once on the *first* transition to public. Subsequent visibility toggles do not change it (option A — stable historical date). The public feed query filters `WHERE published_at IS NOT NULL` so drafts can't leak into it. A v2 "republish" action could reset this explicitly.
- The public feed index is **partial** (`WHERE visibility = 'public' AND published_at IS NOT NULL`) — it's smaller and faster than indexing every row. The per-user index uses `created_at` so the owner sees drafts in their own profile view.
- `width`, `height` on `artwork_images` are required so the masonry layout can reserve space before image bytes arrive (no reflow).
- `blurhash` is a ~30-char string (BlurHash format) computed at upload time, used as the `blurDataURL` placeholder in `next/image`.
- `client_image_id` is a UUID generated by the client per file; it's the *first* idempotency key. The `(artwork_id, client_image_id)` unique constraint guarantees a retry of the same file returns the existing row instead of creating a duplicate.
- `source_sha256` is the *second* idempotency key — a hex SHA-256 of the original bytes computed at upload time. The pipeline accepts a retry only when both `client_image_id` *and* `source_sha256` match the existing row. A retry that reuses the same `client_image_id` with different bytes is rejected (409 Conflict) instead of silently returning the old row. Without this column the server cannot tell "true retry" from "buggy client reusing the key."
- `position` is supplied by the client (not "next free"), so retries land at the same position the original attempt used. The `(artwork_id, position)` unique constraint catches client bugs like reusing a position with a different `client_image_id`.
- `cover_position` chooses which image is the artwork's cover by referring to its `position` value, not its `id`. We previously held `cover_image_id uuid` with an FK to `artwork_images(id)`, but the FK only enforces existence — a buggy update could point Artwork A's cover at an image owned by Artwork B. The current representation makes that bug structurally impossible: the cover is by definition the row where `(artwork_id, position) = (artwork_id, cover_position)`. Default `0` means the first uploaded image is the cover automatically; an explicit `PATCH /artworks/:id { "cover_position": N }` lets the artist choose otherwise.
- `storage_key` is `<visibility>/<artwork_id>/<image_id>.<ext>` — uses the image's own UUID, **not** position. Reordering becomes `UPDATE artwork_images SET position = ?` with no R2 moves.

## 6. Backend (Go)

### 6.1 Project layout

```
cmd/api/main.go              - server bootstrap, config, dependency wiring
internal/
  httpapi/                   - chi router, handlers, error→HTTP mapping
  auth/                      - OAuth flow, JWT issue/verify, middleware
  user/                      - user service + repository
  artwork/                   - artwork service + repository, feed query
  image/                     - upload pipeline (validate → decode → blurhash → store)
  storage/                   - Storage interface + localfs.Store + r2.Store
  db/                        - pgx pool, query helpers
migrations/                  - SQL migrations (golang-migrate)
```

### 6.2 Storage interface

```go
type Storage interface {
    Put(ctx context.Context, key string, body io.Reader, contentType string) error
    SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
    PublicURL(key string) string
    Delete(ctx context.Context, key string) error
    Move(ctx context.Context, srcKey, dstKey string) error
}
```

`localfs.Store` writes under `./var/storage/`; in dev a small file-server handler exposes them at `http://localhost:8080/dev-cdn/...` so the frontend can use the same URL shape.

`r2.Store` uses the AWS S3 SDK pointed at the R2 endpoint with R2 credentials. `Move` is a copy + delete (R2 has no native move).

### 6.3 HTTP endpoints

```
POST   /auth/google/start            → 302 to Google
GET    /auth/google/callback         → exchange code, set cookie, redirect to FE
POST   /auth/logout                  → clear cookie
GET    /me                           → current user (or 401)

POST   /artworks                     → create artwork (visibility=private = effectively a draft)
POST   /artworks/:id/images          → multipart upload (1..N files), idempotent on (artwork_id, client_image_id); see §6.5
PATCH  /artworks/:id                 → update title/description/visibility/tags
DELETE /artworks/:id

GET    /artworks?cursor=&limit=24    → public feed (cursor-paginated)
GET    /artworks/:id                 → detail; 404 if private and not owner
GET    /users/:slug                  → profile + artworks (filtered by viewer)
GET    /tags/:name?cursor=&limit=24  → public artworks for tag
```

All JSON. Cursor is base64'd `(published_at, id)` tuple.

### 6.4 Auth flow

1. `GET /auth/google/start` (Go) → 302 to Google with `state` cookie.
2. Google → `GET /auth/google/callback` with `code` and `state`.
3. Go validates state, exchanges code for tokens, fetches Google profile.
4. Go upserts `users` row keyed by `(oauth_provider, oauth_subject)`.
5. Go issues an HS256 JWT with `{ sub: user_id, exp: now + 7d }`.
6. Go sets cookie: `Set-Cookie: auth=<jwt>; HttpOnly; Secure; SameSite=Lax; Domain=.example.com; Path=/; Max-Age=604800`.
7. Go 302s to `https://example.com/`.
8. Subsequent requests carry the cookie. Middleware verifies JWT, attaches `user_id` to context, or returns 401 for protected routes.

**Provider extensibility.** Routes are parameterized: `/auth/{provider}/start` and `/auth/{provider}/callback` (where `{provider}` is a chi URL parameter). The auth package exposes a `Provider` interface:

```go
type Provider interface {
    Name() string                                          // "google", "apple", ...
    AuthURL(state string) string                           // builds redirect URL
    Exchange(ctx context.Context, code string) (*Profile, error)
}
```

Adding Apple, Facebook, Instagram, GitHub, etc. is a one-line registration in `main.go` plus a new provider implementation; no handler or route changes. The `users.oauth_provider` column already supports an arbitrary string discriminator.

### 6.5 Upload pipeline (`POST /artworks/:id/images`)

**Request shape** — multipart form with a JSON manifest plus the binary parts:

```
POST /artworks/:id/images
Content-Type: multipart/form-data

field "manifest" (application/json):
  [
    { "client_image_id": "<uuid>", "position": 0, "content_type": "image/jpeg" },
    { "client_image_id": "<uuid>", "position": 1, "content_type": "image/png" },
    ...
  ]

field "files":
  [ file_0, file_1, ... ]   // matched by index to the manifest entries
```

The client generates `client_image_id` (UUID v4) per file and decides each file's `position` (the order within the artwork). Both must be supplied; the server never picks "next free."

**Per-file processing — runs in its own DB transaction so partial success across files is fine:**

1. Verify viewer owns artwork; reject if not.
2. Validate `Content-Type` ∈ {`image/jpeg`, `image/png`} and size ≤ 25 MB. Reject mismatches.
3. Read the source bytes; compute `source_sha256 = hex(sha256(bytes))`. Computing the fingerprint up front is what makes step 4 sound.
4. `SELECT id, source_sha256 FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`.
   - If found *and* the stored `source_sha256` matches the just-computed value: this is a true retry. Return the existing row and skip steps 5-10 entirely.
   - If found *and* the stored `source_sha256` does **not** match: the client reused the same `client_image_id` for different bytes. Return **409 Conflict** with `{"error":"fingerprint_mismatch"}`. Do not overwrite — the original row stays as-is.
5. Decode image to measure `width`, `height` (`image.Decode` from stdlib).
6. Compute `blurhash` from a downscaled version.
7. Generate a new image `id` (UUID v4) for this row.
8. Build storage key: `<visibility>/<artwork_id>/<image_id>.<ext>`.
9. `storage.Put(key, originalBody, contentType)` — bytes are not re-encoded; we stream the original.
10. `INSERT INTO artwork_images ...` (including `source_sha256`).
    - If `(artwork_id, client_image_id)` unique violation: another concurrent retry won the race. `SELECT` the winning row, re-check its `source_sha256` against the just-computed value (same fingerprint logic as step 4), and either return the existing row or return 409. The duplicate R2 PUT is a leaked object — acceptable, see §11.
    - If `(artwork_id, position)` unique violation with a *different* `client_image_id`: client bug (a different image already occupies that position). Return 412 Precondition Failed.

`cover_position` defaults to `0` at artwork creation, so the first image — which lands at `position = 0` — is the cover automatically. No explicit `UPDATE artworks SET cover_position = ?` is required at upload time. Artists can pick a different cover via `PATCH /artworks/:id { "cover_position": N }`.

We do **not** generate thumbnails on upload. The Cloudflare image transform creates them on demand.

**Why this is safe to retry:**

- Retrying the whole batch with the same `client_image_id`s and the same files skips already-uploaded images at step 4.
- Retrying after a partial failure where 3 of 5 succeeded: 3 are short-circuited at step 4 (true retry); 2 proceed normally.
- Concurrent retries of the same image: one wins, the other returns the same row from the loser's catch path at step 10.
- A buggy client that reuses a `client_image_id` with different bytes does not silently overwrite or get a misleading "OK already uploaded" — it gets a 409 telling it to use a fresh `client_image_id`.
- Position collisions only happen when the client mis-uses the API; they never corrupt server state.

### 6.6 Privacy state changes

`PATCH /artworks/:id { visibility: "private" }` (going public → private):

1. Open DB transaction.
2. For each `artwork_image`, `storage.Move(public/<artwork_id>/<image_id>.<ext>, private/<artwork_id>/<image_id>.<ext>)` (R2 copy + delete).
3. `UPDATE artwork_images.storage_key` rows.
4. `UPDATE artworks SET visibility = 'private'`. **`published_at` is NOT cleared** (option A).
5. Commit.
6. Async: purge old `/public/...` paths (and their transform variants) from CDN cache via Cloudflare API.

`PATCH /artworks/:id { visibility: "public" }` (going private → public):

1. Open DB transaction.
2. For each `artwork_image`, `storage.Move(private/..., public/...)`.
3. `UPDATE artwork_images.storage_key` rows.
4. `UPDATE artworks SET visibility = 'public', published_at = COALESCE(published_at, now())`. The `COALESCE` enforces option A: set on first publish only.
5. Commit.

**Failure-mode analysis (both directions).** Storage moves are non-transactional with the DB, so any flip can fail mid-flight. The implementation MUST roll back already-moved objects on any error before returning, otherwise:

- *`public → private` partial:* one image at `/private/`, one still at `/public/`, DB still says public. Old `/public/` URLs continue to work for the still-moved image; the moved image returns 404 because no signed URL was issued for `/private/` yet. Visible damage is bounded — the artwork stays public (some images broken) and no private content leaks. Acceptable on its own, but tests assert no leaks anyway.
- *`private → public` partial without rollback:* one image at `/public/`, one still at `/private/`, DB still says private. **The image at `/public/` is now served unsigned to anyone who knows its path** — the Worker authorizes by path prefix, not by DB state. This is a privacy regression: the artwork is supposed to be private, but bytes are reachable as if public.
- *DB commit fails after all moves succeed:* every image has been moved to the target prefix; rollback restores them so the DB state and storage state stay in sync.

The rollback path is best-effort. If reversing a move itself fails (network, R2 outage), a misplaced object stays at the destination prefix — handled by the v2 R2 GC sweeper (§11) and surfaced via `RollbackLog` in the meantime.

`public → private` was historically called the "dangerous" direction in earlier drafts; with the rollback in place both directions are safe under partial failure. The remaining unsafe scenario is "rollback itself fails after a `private → public` partial move," which the GC sweeper closes within its scan window.

## 7. Frontend (Next.js)

### 7.1 Routes

```
app/
  layout.tsx                   - root: theme, top nav (logo, browse, login/avatar)
  page.tsx                     - home feed (server component; masonry, public artworks)
  u/[slug]/page.tsx            - artist profile
  art/[id]/page.tsx            - artwork detail
  tag/[name]/page.tsx          - public artworks for a tag
  upload/page.tsx              - upload form (auth-gated)
  settings/page.tsx            - profile edit (auth-gated)
components/
  Masonry.tsx                  - CSS-columns gallery
  ArtCard.tsx                  - thumbnail with blurhash placeholder
  InfiniteFeed.tsx             - intersection-observer pagination wrapper
  ArtworkUploader.tsx          - multi-file upload form
lib/
  api.ts                       - typed fetch helpers; forwards cookies on SSR
  cf-loader.ts                 - custom next/image loader; appends w/fmt/q to the Worker /img URL
  blurhash.ts                  - BlurHash → tiny base64 PNG
```

### 7.2 Image rendering

```tsx
<Image
  loader={cfLoader}
  src={image.url}              // origin URL from API (public or signed private)
  width={image.width}
  height={image.height}
  alt={artwork.title}
  placeholder="blur"
  blurDataURL={blurhashToDataURL(image.blurhash)}
  sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 25vw"
/>
```

`cfLoader` appends three transform query params (`w`, `fmt`, `q`) to the Worker URL, picking the requested width from the **allowlist** (see §8.4). See §8.4 for the loader code.

`next.config.js` overrides Next's default `deviceSizes` to align with the allowlist so the auto-generated `srcset` only requests valid widths:

```js
// next.config.js
module.exports = {
  images: {
    deviceSizes: [240, 480, 800, 1024, 1600, 2400],
    imageSizes: [],
    loaderFile: './lib/cf-loader.ts',
  },
};
```

The combination of:
- `width`/`height` reserving aspect-ratio space,
- `blurDataURL` rendering a placeholder before bytes arrive,
- `sizes` driving an automatic `srcset` constrained to the allowlist,
- `fmt=auto` serving AVIF/WebP where supported (Worker negotiates from `Accept` header),
- `loading="lazy"` (default in `next/image`),

is what produces the smooth, no-jank gallery feel.

### 7.3 Masonry

```css
.masonry { columns: 4 240px; column-gap: 12px; }
.masonry > * { break-inside: avoid; margin-bottom: 12px; }
```

CSS multi-column layout. No JS. `4 240px` means "as many columns as fit, each at least 240px wide, target 4."

### 7.4 Infinite scroll

The feed page is a server component that fetches page 1 from `GET /artworks?limit=24` during SSR — first paint has real content, no loading state. A small client component at the bottom uses `IntersectionObserver` to fetch `?cursor=<last>` and append. We prefetch one page ahead so scrolling never hits a loading state.

### 7.5 Auth on the FE

Login button is a plain `<a href="https://api.example.com/auth/google/start">`. The Go API sets the cookie on `.example.com`, so both the API and the FE see it. SSR fetches in Next forward the request's `Cookie` header to the Go API. CORS on Go API allows credentials from the Next origin only.

There is no NextAuth, no JWT in localStorage, no auth client SDK on the frontend.

### 7.6 Cache control for pages that may include private content

Next.js's default rendering behaviors are aggressive caching — both static rendering at build time and the Data Cache for fetches. **Any page whose HTML can include private content for the viewer must opt out of every layer of caching**, otherwise an authenticated owner's private artwork can be served as cached HTML to an anonymous visitor.

The hard rule:

| Route | Caching | Why |
|---|---|---|
| `/` (home feed) | Cacheable (public-only data) | Public feed query filters `visibility='public' AND published_at IS NOT NULL`; safe to cache. |
| `/tag/[name]` | Cacheable (public-only data) | Tag pages are public-only by spec (§8.6 case 5). |
| `/u/[slug]` | **`force-dynamic` + `no-store`** | Owner sees their own drafts; uncacheable. |
| `/art/[id]` | **`force-dynamic` + `no-store`** | Private artwork visible only to owner; uncacheable. |
| `/upload`, `/settings` | **`force-dynamic`** | Auth-gated; uncacheable. |

```ts
// app/u/[slug]/page.tsx
export const dynamic = 'force-dynamic';
export const fetchCache = 'force-no-store';

// app/art/[id]/page.tsx
export const dynamic = 'force-dynamic';
export const fetchCache = 'force-no-store';
```

Additionally, server-side fetches to the Go API must pass `{ cache: 'no-store' }` whenever the response could embed signed private URLs. Cached signed URLs are a double leak: stale auth, served to the wrong viewer.

For the Cloudflare CDN sitting in front of Next.js HTML responses (if applicable):
- Cache rules must respect `Cache-Control: private` and `Cache-Control: no-store` headers from origin.
- Never cache responses that carry `Set-Cookie`.
- For `cdn.example.com/img/*` (the Worker route), the Worker sets `Cache-Control` and a custom `cf.cacheKey` explicitly per request — see §8.3. Public images cache aggressively at the edge; private images use `private, no-store` and are never edge-cached.

## 8. Privacy & Access Enforcement

### 8.1 Storage access path

```
Browser
  │
  ▼  GET cdn.example.com/img/<storage_key>?sig=…&exp=…&w=800&fmt=auto&q=85
┌─────────────────────────────┐
│   Cloudflare Worker         │
│   (single chokepoint)       │
│                             │
│   1. Validate path prefix   │
│   2. Validate sig (private) │
│   3. Validate transform     │
│      params against allow-  │
│      list (DoS hardening)   │
│   4. R2.get(key)            │ ◀── R2 binding
│   5. IMAGES.transform(...)  │ ◀── env.IMAGES binding
│   6. Set Cache-Control +    │
│      cf.cacheKey            │
│   7. Return                 │
└─────────────────────────────┘
       │
       ▼
   ┌─────────┐
   │  R2     │  (private bucket — no public access; only Worker holds binding)
   │ public/ │
   │ private/│
   └─────────┘
```

**The R2 bucket has no public-read policy and no custom domain.** The Worker is the only caller in the system that can read its bytes (via R2 binding). No HTTP path leads to R2 except through the Worker. The Worker is the *only* place that decides whether a request gets bytes back.

This eliminates the routing ambiguity of the previous `/cdn-cgi/image/...` design — Cloudflare's image-resizing service is not in the request path; transform happens *inside* the Worker via the `env.IMAGES` binding.

### 8.2 Canonical signing scheme

The API and the Worker must agree byte-for-byte on the string they sign. Mismatch = owner can't load their own private images, and any sloppy fix risks a security hole.

**Canonical "string-to-sign":**

```
v1|<canonicalPath>|<exp>
```

where `<canonicalPath>` is:

- always one of `/private/<rest>` (leading slash, no `/img/` prefix — that's a routing concern, not signing)
- alphanumeric + `/` + `.` + `_` + `-` only — storage keys never contain URL-encoded chars because UUID and integer position are both ASCII-safe
- case-sensitive
- no `//`, no `..`, no `./`, no trailing slash

The version prefix `v1` lets us evolve the signing scheme later without ambiguity. `<exp>` is a Unix timestamp (seconds, base-10 digits, no padding).

**The signature covers the source key only — not the transform params (`w`, `fmt`, `q`).** Those control output rendering, not access. Anyone with a valid `sig` for the source can render any allowed size; this matches the standard CDN-image model and avoids requiring a separate API call per viewport breakpoint. The allowlist (§8.3) bounds what variants exist, so this isn't a DoS surface.

### 8.3 Worker logic

```js
// worker/src/index.js
const IMG_PREFIX = "/img/";
const ALLOWED_WIDTHS  = new Set([240, 480, 800, 1024, 1600, 2400]);
const ALLOWED_FORMATS = new Set(["auto", "avif", "webp", "jpeg"]);
const ALLOWED_QUALITIES = new Set([60, 75, 85, 90]);

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    if (!url.pathname.startsWith(IMG_PREFIX)) {
      return new Response("Not found", { status: 404 });
    }
    const canonicalPath = "/" + url.pathname.slice(IMG_PREFIX.length);  // "/private/<id>/0.jpg"

    // Reject canonicalization-ambiguous paths.
    if (canonicalPath.includes("//") || canonicalPath.includes("/../") || canonicalPath.includes("/./")) {
      return new Response("Bad request", { status: 400 });
    }

    // Auth gate by prefix.
    let isPrivate;
    if (canonicalPath.startsWith("/private/")) {
      isPrivate = true;
      const sig = url.searchParams.get("sig");
      const expStr = url.searchParams.get("exp");
      if (!sig || !expStr) return new Response("Unauthorized", { status: 401 });
      const exp = parseInt(expStr, 10);
      if (!Number.isFinite(exp) || Date.now()/1000 > exp) {
        return new Response("Unauthorized", { status: 401 });
      }
      const expected = await hmacHex(env.WORKER_SIGNING_KEY, `v1|${canonicalPath}|${exp}`);
      if (!constantTimeEqual(sig, expected)) {
        return new Response("Unauthorized", { status: 401 });
      }
    } else if (canonicalPath.startsWith("/public/")) {
      isPrivate = false;
    } else {
      return new Response("Not found", { status: 404 });
    }

    // Transform params — clamped to the allowlist (DoS hardening + cache-key bounding).
    const wRaw = url.searchParams.get("w");
    const w = wRaw === null ? null : parseInt(wRaw, 10);
    if (w !== null && !ALLOWED_WIDTHS.has(w))   return new Response("Bad request: w not in allowlist", { status: 400 });

    const fmt = url.searchParams.get("fmt") ?? "auto";
    if (!ALLOWED_FORMATS.has(fmt))               return new Response("Bad request: fmt not in allowlist", { status: 400 });

    const qRaw = url.searchParams.get("q");
    const q = qRaw === null ? 85 : parseInt(qRaw, 10);
    if (!ALLOWED_QUALITIES.has(q))               return new Response("Bad request: q not in allowlist", { status: 400 });

    // Read source from R2.
    const r2Key = canonicalPath.slice(1);
    const obj = await env.R2.get(r2Key);
    if (!obj) return new Response("Not found", { status: 404 });

    // Transform.
    const outputFormat = fmt === "auto" ? negotiateFormat(request) : `image/${fmt}`;
    let pipeline = env.IMAGES.input(obj.body);
    if (w !== null) pipeline = pipeline.transform({ width: w });
    const transformed = (await pipeline.output({ format: outputFormat, quality: q })).response();

    // Cache headers + custom cache key (excludes sig/exp so private never collides
    // with public; see §8.4 for the cf.cacheKey rationale).
    const headers = new Headers(transformed.headers);
    if (isPrivate) {
      headers.set("Cache-Control", "private, no-store");
    } else {
      headers.set("Cache-Control", "public, max-age=31536000, immutable");
    }
    return new Response(transformed.body, { status: transformed.status, headers });
  },
};

function negotiateFormat(request) {
  const accept = request.headers.get("Accept") || "";
  if (accept.includes("image/avif")) return "image/avif";
  if (accept.includes("image/webp")) return "image/webp";
  return "image/jpeg";
}
```

`constantTimeEqual` is required (not `===`) to prevent timing-attack signature recovery.

**Why the allowlists matter:**

- **Widths** — without clamping, any attacker could request `?w=99999` and force the Worker to allocate a huge image. The allowlist also bounds the cache-key cardinality: at most 6 widths × 4 formats × 4 qualities = 96 cache entries per source image, not unbounded.
- **Formats** — `env.IMAGES` would error on unknown formats, but explicit rejection gives a cleaner 400 and keeps the test surface small.
- **Qualities** — same DoS concern as widths; also stops accidental `?q=100` bandwidth bloat.

### 8.4 URL generation (Go API + Next.js loader)

**Split of responsibility:**

- **API** returns the origin URL — `cdn.example.com/img/<key>` plus `sig` + `exp` for private. No transform params.
- **Frontend** (`cf-loader`) appends `w`, `fmt`, `q` per render, picking `w` from the allowlist nearest-up to what `next/image` requested.

```go
// API: produce the origin URL the frontend will hand to next/image as `src`.
//
// storageKey is the value stored in artwork_images.storage_key, e.g.
// "private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0.jpg" — no leading slash.

func (s *Service) PublicImageURL(storageKey string) string {
    return "https://cdn.example.com/img/" + storageKey
}

func (s *Service) PrivateImageURL(storageKey string) string {
    canonicalPath := "/" + storageKey                  // signed path, no /img/ prefix
    exp := time.Now().Add(5 * time.Minute).Unix()
    stringToSign := fmt.Sprintf("v1|%s|%d", canonicalPath, exp)
    mac := hmac.New(sha256.New, s.signingKey)
    mac.Write([]byte(stringToSign))
    sig := hex.EncodeToString(mac.Sum(nil))
    return fmt.Sprintf("https://cdn.example.com/img%s?sig=%s&exp=%d", canonicalPath, sig, exp)
}
```

```ts
// Frontend: lib/cf-loader.ts
const ALLOWED_WIDTHS = [240, 480, 800, 1024, 1600, 2400];

function pickWidth(requested: number): number {
  for (const w of ALLOWED_WIDTHS) if (w >= requested) return w;
  return ALLOWED_WIDTHS[ALLOWED_WIDTHS.length - 1];
}

export default function cfLoader({ src, width, quality }:
  { src: string; width: number; quality?: number }): string {
  const u = new URL(src);
  u.searchParams.set("w", String(pickWidth(width)));
  u.searchParams.set("fmt", "auto");
  u.searchParams.set("q", String(quality ?? 85));
  return u.toString();
}
```

**Cache-key safety (resolves Finding 2).** The Worker must explicitly set a custom `cf.cacheKey` for *public* images that omits `sig`/`exp` (which are absent for public anyway, but explicit > implicit). For private images, `Cache-Control: private, no-store` keeps the response out of any shared cache. There is no path by which an unsigned request to `/img/private/...` can hit a cached owner-authored response because the Worker runs auth before any cache lookup it might consult, and `no-store` means there's nothing to lookup in the first place.

Five-minute signature TTL is the trade-off between leakage window (a leaked URL is valid for ≤5 min) and re-fetch frequency (`next/image` re-validation handles refresh transparently).

### 8.5 Information leakage rules

- A request for a private artwork by a non-owner returns **404**, not 403. The handler must treat "private and not yours" identically to "doesn't exist" so non-owners can't infer the existence of private works.
- A direct request to the CDN for a private path without a signature returns 401, not 403, for the same reason.
- API error messages must not echo whether an artwork ID exists when the viewer cannot see it.

### 8.6 Critical correctness tests

These run on every commit; failure blocks merge. Five test categories below — privacy matrix, HMAC canonicalization + cache safety, upload idempotency, publish lifecycle, and frontend UX — all share the same property: silent regression leaks private content, corrupts data, or breaks the smooth-gallery promise.

#### 8.6.1 Privacy matrix (all surfaces × viewer types)

The previous spec only tested *direct access* (artwork detail and CDN paths). Private artwork can leak through every list/discovery surface — feed, profile, tag, SSR HTML — and through Next.js's caching layers. This matrix is the canonical authoritative test set.

**Setup** (used by every row): user **A** owns artwork **P** (public, tagged `t`) and artwork **Q** (private, tagged `t`). User **B** is a different signed-in user. **Anon** is unauthenticated.

| # | Surface | Anonymous | Owner (A) | Other auth (B) |
|---|---|---|---|---|
| 1 | `GET /artworks` (public feed JSON) | 200; contains P; **must not** contain Q; **no signed `/img/private/` URLs in payload** | 200; contains P; **must not** contain Q (this is the public feed even for the owner) | 200; contains P; **must not** contain Q |
| 2 | `GET /artworks/:id` where `id=Q` | 404 | 200; payload contains `cdn.example.com/img/private/...?sig=...&exp=...` | 404 (not 403; 404 hides existence) |
| 3 | `GET /artworks/:id` where `id=P` | 200; `cdn.example.com/img/public/...` (no `sig`/`exp`) | 200; public URL | 200; public URL |
| 4 | `GET /users/:slug` (A's profile JSON) | 200; lists P; **must not** list Q; **no signed URLs in payload** | 200; lists P **and** Q; Q's images carry signed URLs | 200; lists P; **must not** list Q |
| 5 | `GET /tags/:name` where `name=t` | 200; contains P; **must not** contain Q (tag pages are public-only by design) | 200; contains P; **must not** contain Q | 200; contains P; **must not** contain Q |
| 6 | SSR `/` (home page HTML) | HTML contains no `Q.id`, no `private/`, no `sig=` query string | same | same |
| 7 | SSR `/u/:slug` (HTML, A's profile) | HTML contains no `Q.id`, no `private/`, no `sig=` | HTML contains `Q.id`, signed `/img/private/` URLs, `sig=` query strings | HTML contains no `Q.id`, no `private/`, no `sig=` |
| 8 | SSR `/tag/:name` (HTML) | HTML contains no `Q.id`, no `private/`, no `sig=` | same | same |
| 9 | SSR `/art/:id` HTML where `id=Q` | renders 404 page; **HTML body must not contain any reference to Q** (no title, no description) | renders 200 page with signed URLs | renders 404 page; same scrubbing as anonymous |
| 10 | Worker `cdn.example.com/img/public/<key>?w=800` (direct GET) | 200; `Content-Type: image/avif`/`webp`/`jpeg`; **response width within 1px of 800** (proves resize ran); `Cache-Control: public, max-age=31536000, immutable` | 200 | 200 |
| 11 | Worker `cdn.example.com/img/private/<key>?w=800` (no `sig`) | 401 | 401 (Worker auth is by signed URL, not cookies) | 401 |
| 12 | Worker `cdn.example.com/img/private/<key>?sig=valid&exp=future&w=800` | 200; **response width within 1px of 800**; `Cache-Control: private, no-store` | 200 | 200 (any holder of a valid signed URL gets through — by design; 5-min TTL) |
| 13 | Worker `cdn.example.com/img/private/<key>?sig=valid&exp=future&w=99999` (out-of-allowlist width) | 400 | 400 | 400 |
| 14 | Worker URL with valid `sig` but `fmt=svg` (out-of-allowlist) | 400 | 400 | 400 |

**Negative-content assertions** (cases 1, 4, 6, 7, 8, 9 anon/B columns): the test must not just check that Q's row is absent. It must:

- Search the JSON/HTML response body for the literal `private/` substring → must not appear.
- Search for any occurrence of `Q.id` (the artwork's UUID) → must not appear.
- Search for `sig=` or `exp=` query parameters → must not appear.

This catches subtle leaks like: API forgot to filter Q from the feed → row appears in JSON; or SSR component rendered Q but with `display:none` CSS → still in HTML; or response includes related-artwork links that don't honor visibility.

**Resize verification** (cases 10, 12 — width assertion): a 200 response is not enough. The test must decode the response body (e.g., `Buffer` → `sharp().metadata()` in Node, or `image.Decode` in Go) and assert the actual decoded width is within 1 pixel of the requested `w`. This catches "Worker returned the original 4000px image without applying the transform" — a class of bug where the test passes superficially but the resize never ran. Originals stored in R2 are intentionally larger than any allowlist width, so a missed transform shows up as an obvious size mismatch.

**Next.js cache regression test** (specific case 7 anon column): after an *owner* request hits `/u/:slug` (which would populate any naïve cache with owner-visible HTML), make an *anonymous* request to the same URL and re-verify the negative-content assertions. This catches the Next.js Data Cache footgun called out in §7.6.

#### 8.6.2 HMAC canonicalization & cache-safety round-trips (cases 15–16)

15. **Signed-URL round-trip.** Test code calls Go `PrivateImageURL("private/<id>/0.jpg")`, then issues `GET cdn.example.com/img/private/<id>/0.jpg?sig=...&exp=...&w=800&fmt=auto&q=85` against a deployed dev Worker and asserts (a) HTTP 200 and (b) the response decodes to a JPEG/WebP/AVIF image with width within 1px of 800. Catches both canonicalization drift between API and Worker *and* "Worker forgot to call IMAGES.transform". Runs in CI on every commit touching either side, against a real Worker.
16. **Cache-key safety (Finding 2 regression).** Owner GETs `cdn.example.com/img/private/<key>?sig=valid&exp=future&w=800` → 200. **Then** anonymous GETs the *same path with no `sig`/`exp`* → 401. This proves the Worker does not return a cached owner-bytes response to an unsigned request, regardless of any path-only cache key misconfiguration. Repeats for an owner-then-attacker variation where the attacker submits a tampered `sig` — also expects 401.

#### 8.6.3 Upload idempotency (cases 17–20, plus fingerprint regression 17b)

17. **Retry with the same `client_image_id` and same bytes is safe.** POST one image with `client_image_id=K`, position=0, file=`cat.jpg`. POST again with the *same* `client_image_id`, *same* position, and *same* file. Second response returns the *same* `artwork_image.id` as the first. `SELECT count(*) FROM artwork_images WHERE artwork_id=A` returns 1, not 2.
17b. **Same `client_image_id` with different bytes is rejected (409 fingerprint mismatch).** POST `client_image_id=K`, position=0, file=`cat.jpg`. Then POST `client_image_id=K`, position=0, file=`dog.jpg`. Second response is **409 Conflict** with `{"error":"fingerprint_mismatch"}`. DB state unchanged: `cat.jpg` is still the row at position 0; `dog.jpg` was not stored. Catches buggy clients that reuse upload tokens for different files — without the fingerprint check, the server would silently return the original row and the client would think the new file uploaded successfully.
18. **Duplicate position is rejected.** With image at position=0 (`client_image_id=K1`) already inserted, POST a *different* image (`client_image_id=K2`) at position=0. Response is 412 Precondition Failed; DB state unchanged.
19. **Concurrent uploads do not create duplicate positions.** Spawn N=10 parallel POSTs against the same artwork, each with a unique `client_image_id` and the same position=5. Exactly one returns 200; the other 9 return 412. `SELECT count(*) FROM artwork_images WHERE artwork_id=A AND position=5` = 1.
20. **Failed third image leaves first two valid; user retries the third.** POST 5 files; simulate a server panic on the 3rd file mid-processing. Verify rows for files 0, 1 exist (their per-file transactions committed). Re-POST all 5 files with the same `client_image_id`s *and the same bytes*. Files 0, 1 return their existing rows (true retry — fingerprints match); files 2, 3, 4 are processed and inserted. Final state = 5 rows, no duplicates.

#### 8.6.4 Publish lifecycle (case 21)

21. **`published_at` semantics (option A).** Create artwork with `visibility='private'`; verify `published_at IS NULL` and the artwork does *not* appear in `GET /artworks`. Flip to `public`; verify `published_at` is now `~= now()` and the artwork appears in the feed. Flip private→public→private→public again; verify `published_at` is unchanged from the first public transition.

#### 8.6.5 Frontend UX correctness (cases 22–25)

These run as Playwright E2E tests against a docker-compose'd full stack on every commit.

22. **Infinite scroll does not duplicate items.** Load `/`, scroll to trigger 5 page fetches (~120 items). Collect all rendered `data-artwork-id` attributes; assert each appears exactly once. Catches cursor-pagination off-by-one bugs and React-key collision bugs.
23. **Masonry reserves dimensions; no layout jump.** Load `/` with network throttled to "Slow 3G." Take a screenshot at 0ms (placeholders only) and at 5000ms (images loaded). Assert the bounding boxes of all visible cards have not moved (Cumulative Layout Shift < 0.05 over the load). Catches missing `width`/`height` on `next/image` and broken aspect-ratio reservation.
24. **Lazy loading only requests images near the viewport.** Load `/` with network logging. Initially, only images within the first 2 viewport heights should have requested image bytes (look for `cdn.example.com/img/...` requests). Scroll down 3 viewport heights; verify additional image requests now appear. Catches accidental `loading="eager"` or broken intersection-observer.
25. **Flip private + incognito = disappearance.** Owner signs in, posts artwork P (public). Verify P appears at `/u/<slug>` in an incognito window. Owner flips P to private. Within ~30s (allowing CDN purge), reload the incognito window and assert P no longer appears at `/u/<slug>`, `/`, `/tag/<name>`, or `/art/<id>` (the last must show the 404 page with no leaked metadata).

## 9. Testing Strategy

### 9.1 Go (`go test`)

| Layer | What | How |
|---|---|---|
| Repositories | DB queries, constraints, unique violations, indexes | Real Postgres via testcontainers; fresh schema per test |
| Services | Business logic (privacy flip, upload pipeline, feed query) | Real DB + in-memory `Storage` fake for speed |
| HTTP handlers | Routing, auth middleware, status codes, error mapping | `httptest` against the full chain with real DB |
| Auth | OAuth callback, JWT issue/verify, cookie attributes | Mock Google's token endpoint; everything else real |
| Storage impls | `localfs.Store` and `r2.Store` | Real filesystem for localfs; testcontainers MinIO for r2 |
| Critical correctness | All 26 cases in §8.6 (privacy matrix 14, HMAC + cache-safety round-trip 2, upload idempotency 5 incl. 17b fingerprint mismatch, publish lifecycle 1, frontend UX 4) | Privacy matrix: `internal/artwork/privacy_matrix_test.go` (Go) + `e2e/privacy_html.spec.ts` (Playwright). Worker round-trips: `worker/test/round_trip.spec.ts` (against deployed dev Worker). Upload: `internal/image/upload_idempotency_test.go`. Publish: `internal/artwork/publish_lifecycle_test.go`. Frontend UX: `e2e/ux.spec.ts`. |

**Rule:** integration tests use real Postgres, never a mock. (Mocked DB tests pass while the real migration breaks — burned by this before.)

### 9.2 Next.js (Vitest + Playwright)

| Layer | What | How |
|---|---|---|
| Pure utils | `cf-loader`, `blurhashToDataURL`, cursor parsing | Vitest unit |
| Components | `Masonry`, `ArtCard` placeholder, `InfiniteFeed` observer | Vitest + RTL with mocked API responses |
| E2E | Login → upload → view → flip private → log out → confirm 404 | Playwright against docker-compose'd full stack |

Don't write tests at all three layers for the same code — pick the right level.

## 10. Operational Concerns

- **Logging**: structured (zap or slog). Every privacy decision (public served, private served, private rejected) gets a log line for audit.
- **Metrics**: request count + latency by endpoint, image upload count, R2 PUT/GET count, CDN cache hit ratio.
- **Config**: env vars only. No file-based config. Twelve-factor.
- **Secrets**: `GOOGLE_OAUTH_CLIENT_SECRET`, `JWT_SIGNING_KEY`, `WORKER_SIGNING_KEY` (shared with the Cloudflare Worker), `R2_ACCESS_KEY_SECRET`. Loaded from env.
- **Deployment**: Go API as a single static binary in a minimal container; Next.js on Vercel or a Node container; Postgres managed (Neon, Supabase, RDS); R2 + CDN on Cloudflare.

## 11. Risks & Open Questions

- **Cloudflare Worker maintenance burden**: it's a small piece of infra outside the main repos and now does more (auth, R2 read, IMAGES transform, cache-header authoring). Mitigation: keep it under ~120 lines with the allowlist constants pulled to the top, and gate every change on the §8.6 round-trip + cache-safety tests against a deployed dev Worker.
- **Cloudflare Images binding cost**: `env.IMAGES` requires the Cloudflare Images product. Pricing as of writing: $5/mo flat + ~$1 per 1000 transforms; free tier covers ~5000 transforms/month, more than enough for a v1 portfolio. If we ever need to drop this dependency, the fallback is "pre-generate the 6 allowlist size variants at upload time and have the Worker just serve bytes" — adds ~3 seconds to upload latency and ~3x R2 storage, but no runtime image dependency. The Worker code structure (allowlist + auth + R2 read + return) is the same in both designs; only the transform step changes.
- **Next.js caching is a privacy footgun**: the framework's defaults (static rendering at build time, the Data Cache for fetches) mean a single missing `dynamic = 'force-dynamic'` directive on `/u/:slug` or `/art/:id` will serve cached owner-visible HTML to anonymous viewers. Mitigation: cache-control rules in §7.6 are mandatory, and §8.6 case 7-anon includes a regression test that requests as owner first, then anonymous, to catch this leak class.
- **CDN cache purge latency on privacy flips**: Cloudflare cache purges are fast (<30s) but not instant. A user flipping public→private could see the old cached image for up to that window via the public URL. Documented behavior; acceptable for v1.
- **Orphaned R2 objects from partial upload failures**: if `storage.Put` succeeds but the `INSERT` fails (e.g., concurrent retry already inserted, or a transaction rollback), the bytes sit in R2 with no row pointing at them. They're invisible to API consumers (no signed URL, no `/public/` reference) so privacy is preserved, but they consume storage forever. Mitigation: a periodic GC sweeper job (deferred to v2) walks R2 keys and deletes any whose `<artwork_id>/<image_id>` doesn't match a row in `artwork_images`. At v1 scale this leak is bounded and cheap; not blocking ship.
- **No backpressure on uploads**: a malicious user could upload 1000 25MB images. Mitigation v1: rate-limit uploads per user (10/min). Storage cost cap not in scope.
- **Pagination cursor stability**: cursor is `(published_at, id)`. If a user un-publishes during scroll, results shift but no items are duplicated. Acceptable.
- **Slug conflicts**: first-come-first-served on `users.slug`. If you sign up after someone took your name, you get a numeric suffix. v2 can let you change it.

## 12. Acceptance Criteria

A v1 implementation is complete when:

- A new visitor can land on `example.com`, browse the masonry feed without signing in, click any artwork, and see all images at full display resolution (image transform serves the largest reasonable width for the viewport, e.g. up to `width=2400`, AVIF/WebP, quality=85; the original bytes are kept in R2 but not exposed) with the artist name and date.
- An artist can sign in with Google, upload a multi-image artwork with title/description/tags, see it appear on the feed and on their profile.
- The artist can flip an artwork to private; it disappears from the feed and from their public profile (when viewed in an incognito tab) but remains visible in their authenticated view.
- All 26 critical correctness tests in §8.6 pass (privacy matrix across all surfaces and viewer types, signed-URL round-trip with resize verification, cache-key safety regression, upload idempotency including 17b fingerprint-mismatch, publish lifecycle, frontend UX).
- Lighthouse score on the home feed ≥ 90 for Performance with 50 images on screen on a throttled 4G connection.
