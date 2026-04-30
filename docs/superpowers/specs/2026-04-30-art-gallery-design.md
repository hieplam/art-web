# Art Gallery Web App — Design Spec

**Date:** 2026-04-30
**Status:** Approved (pending implementation plan)
**Owner:** Todd Lam

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
| CDN / image transforms | Cloudflare CDN with `/cdn-cgi/image/` URL transforms |
| Access control on storage | Cloudflare Worker validating signed URLs for private content |
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
          │ <img src=cdn/...>          │ SQL
          ▼                            ▼
┌─────────────────────┐         ┌──────────────────────┐
│  Cloudflare CDN     │         │   PostgreSQL         │
│  cdn.example.com    │         │   (managed or self)  │
│  + Worker (auth)    │         └──────────────────────┘
│  + image transforms │
└─────────┬───────────┘
          │ origin pull (R2 SDK)
          ▼
┌─────────────────────┐
│  Cloudflare R2      │
│  (private bucket)   │
│  /public/...        │
│  /private/...       │
└─────────────────────┘
```

**Data flows:**

- **Browser → Next.js**: HTML page requests, all client-side `fetch` for incremental data.
- **Next.js → Go API**: server-side fetch during SSR, forwarding the request's cookies to authenticate as the user.
- **Browser → CDN**: direct `<img>` requests, never via Next or Go. This is the principal "no lag" lever.
- **CDN → R2**: origin pull. Cached aggressively for `/public/*`, never cached for `/private/*`.
- **Worker decision** at the CDN edge: `/public/*` allowed unconditionally; `/private/*` requires a valid HMAC-signed query.

The API never proxies image bytes. This is the architectural rule that lets the gallery feel fast.

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
  cover_image_id uuid,                                       -- FK below
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

ALTER TABLE artworks
  ADD CONSTRAINT artworks_cover_fk
  FOREIGN KEY (cover_image_id) REFERENCES artwork_images(id);

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
- `client_image_id` is a UUID generated by the client per file; it's how upload retries become idempotent. The `(artwork_id, client_image_id)` unique constraint guarantees a retry of the same file returns the existing row instead of creating a duplicate.
- `position` is supplied by the client (not "next free"), so retries land at the same position the original attempt used. The `(artwork_id, position)` unique constraint catches client bugs like reusing a position with a different `client_image_id`.
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
2. `SELECT id, storage_key FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`. If found, this is a retry — return the existing row, skip processing entirely.
3. Validate `Content-Type` ∈ {`image/jpeg`, `image/png`} and size ≤ 25 MB. Reject mismatches.
4. Decode image to measure `width`, `height` (`image.Decode` from stdlib).
5. Compute `blurhash` from a downscaled version.
6. Generate a new image `id` (UUID v4) for this row.
7. Build storage key: `<visibility>/<artwork_id>/<image_id>.<ext>`.
8. `storage.Put(key, originalBody, contentType)` — bytes are not re-encoded; we stream the original.
9. `INSERT INTO artwork_images ...`.
   - If `(artwork_id, client_image_id)` unique violation: another concurrent retry won the race. `SELECT` and return that row (the duplicate R2 PUT is a leaked object — acceptable, see §11).
   - If `(artwork_id, position)` unique violation with a *different* `client_image_id`: client bug (a different image already occupies that position). Return 412 Precondition Failed.
10. If this is the artwork's first image, `UPDATE artworks SET cover_image_id = ?` in the same transaction.

We do **not** generate thumbnails on upload. The Cloudflare image transform creates them on demand.

**Why this is safe to retry:**

- Retrying the whole batch with the same `client_image_id`s skips already-uploaded images at step 2.
- Retrying after a partial failure where 3 of 5 succeeded: 3 are skipped at step 2; 2 proceed normally.
- Concurrent retries of the same image: one wins, the other returns the same row from the loser's catch path at step 9.
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

If a `public → private` move succeeds but the DB commit fails, the object is "stranded" in `/private/` while the DB still says public — but the Worker will block access because the URL says `/public/...`, which now 404s. Result: image hidden, which is the safe failure mode for privacy.

`public → private` is the dangerous direction. `private → public` does the reverse.

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
  cf-loader.ts                 - custom next/image loader for /cdn-cgi/image/
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

`cfLoader` rewrites the origin URL into a transform URL by inserting the `/cdn-cgi/image/<options>/` prefix between the host and path. See §8.3 for the loader code.

The combination of:
- `width`/`height` reserving aspect-ratio space,
- `blurDataURL` rendering a placeholder before bytes arrive,
- `sizes` driving an automatic `srcset`,
- `format=auto` serving AVIF/WebP where supported,
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
- For `/cdn-cgi/image/*`, cache key is path only (no cookie); this is correct because images are content-addressed and signed-URL-gated.

## 8. Privacy & Access Enforcement

### 8.1 Storage access path

```
Browser ──▶ cdn.example.com ──▶ Cloudflare Worker ──▶ R2 (bucket NOT publicly readable)
```

The R2 bucket has no public-read policy. The Worker is the only caller that holds R2 credentials. Anyone hitting an R2 URL directly gets a 401 from R2.

### 8.2 Canonical signing scheme

The API and the Worker must agree byte-for-byte on the string they sign. Mismatch = owner can't load their own private images, and any sloppy fix risks a security hole.

**Canonical "string-to-sign":**

```
v1|<canonicalPath>|<exp>
```

where `<canonicalPath>` is:

- always one of `/private/<rest>` (leading slash, never a `/cdn-cgi/image/...` prefix)
- alphanumeric + `/` + `.` + `_` + `-` only — storage keys never contain URL-encoded chars because UUID and integer position are both ASCII-safe
- case-sensitive
- no `//`, no `..`, no `./`, no trailing slash

The version prefix `v1` lets us evolve the signing scheme later without ambiguity. `<exp>` is a Unix timestamp (seconds, base-10 digits, no padding).

### 8.3 Worker logic

```js
// worker/src/index.js — runs at every request to cdn.example.com
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    let path = url.pathname;

    // Strip Cloudflare image-transform prefix if present, leaving the canonical path.
    // "/cdn-cgi/image/width=800,format=auto/private/abc/0.jpg" → "/private/abc/0.jpg"
    const TRANSFORM_PREFIX = "/cdn-cgi/image/";
    if (path.startsWith(TRANSFORM_PREFIX)) {
      const after = path.slice(TRANSFORM_PREFIX.length);
      const slash = after.indexOf("/");
      if (slash === -1) return new Response("Bad request", { status: 400 });
      path = after.slice(slash);                         // keeps leading "/"
    }

    // Reject anything that could create canonicalization ambiguity.
    if (path.includes("//") || path.includes("/../") || path.includes("/./")) {
      return new Response("Bad request", { status: 400 });
    }

    if (path.startsWith("/public/")) {
      const obj = await env.R2.get(path.slice(1));       // R2 keys have no leading "/"
      if (!obj) return new Response("Not found", { status: 404 });
      return new Response(obj.body, {
        headers: { "Cache-Control": "public, max-age=31536000, immutable" },
      });
    }

    if (path.startsWith("/private/")) {
      const sig = url.searchParams.get("sig");
      const expStr = url.searchParams.get("exp");
      if (!sig || !expStr) return new Response("Unauthorized", { status: 401 });
      const exp = parseInt(expStr, 10);
      if (!Number.isFinite(exp) || Date.now() / 1000 > exp) {
        return new Response("Unauthorized", { status: 401 });
      }
      const stringToSign = `v1|${path}|${exp}`;
      const expected = await hmacHex(env.WORKER_SIGNING_KEY, stringToSign);
      if (!constantTimeEqual(sig, expected)) {
        return new Response("Unauthorized", { status: 401 });
      }
      const obj = await env.R2.get(path.slice(1));
      if (!obj) return new Response("Not found", { status: 404 });
      return new Response(obj.body, {
        headers: { "Cache-Control": "private, no-store" },
      });
    }

    return new Response("Not found", { status: 404 });
  },
};
```

`constantTimeEqual` is required (not `===`) to prevent timing-attack signature recovery.

### 8.4 URL generation (Go API + Next.js loader)

The split of responsibility:

- **API** returns an *origin* URL — the CDN host plus the canonical path, with `sig` + `exp` query params for private content. No transform options.
- **Frontend** (`cf-loader`) wraps that URL with `/cdn-cgi/image/width=<W>,format=auto,quality=85/...` per render. Cloudflare's image transform preserves query strings, so the Worker still sees `sig`/`exp` after the transform.

This keeps width selection out of the API (which has no idea what viewport will render the image) and keeps signing out of the frontend (which has no access to the signing key).

```go
// API: produce the origin URL the frontend will hand to next/image as `src`.
//
// storageKey is the value stored in artwork_images.storage_key, e.g.
// "private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0.jpg" — no leading slash.

func (s *Service) PublicImageURL(storageKey string) string {
    return "https://cdn.example.com/" + storageKey
}

func (s *Service) PrivateImageURL(storageKey string) string {
    canonicalPath := "/" + storageKey                  // enforce leading slash
    exp := time.Now().Add(5 * time.Minute).Unix()
    stringToSign := fmt.Sprintf("v1|%s|%d", canonicalPath, exp)
    mac := hmac.New(sha256.New, s.signingKey)
    mac.Write([]byte(stringToSign))
    sig := hex.EncodeToString(mac.Sum(nil))
    return fmt.Sprintf("https://cdn.example.com%s?sig=%s&exp=%d", canonicalPath, sig, exp)
}
```

```ts
// Frontend: lib/cf-loader.ts — wraps origin URL with the transform.
export function cfLoader({ src, width, quality }: { src: string; width: number; quality?: number }) {
  const opts = `width=${width},format=auto,quality=${quality ?? 85}`;
  // src example:
  //   "https://cdn.example.com/private/<id>/0.jpg?sig=X&exp=Y"
  // Output:
  //   "https://cdn.example.com/cdn-cgi/image/width=800,format=auto,quality=85/private/<id>/0.jpg?sig=X&exp=Y"
  return src.replace(
    "https://cdn.example.com/",
    `https://cdn.example.com/cdn-cgi/image/${opts}/`,
  );
}
```

Five-minute TTL is the trade-off between leakage window (a leaked URL is valid for ≤5 min) and re-fetch frequency (`next/image` re-validation handles refresh transparently).

### 8.5 Information leakage rules

- A request for a private artwork by a non-owner returns **404**, not 403. The handler must treat "private and not yours" identically to "doesn't exist" so non-owners can't infer the existence of private works.
- A direct request to the CDN for a private path without a signature returns 401, not 403, for the same reason.
- API error messages must not echo whether an artwork ID exists when the viewer cannot see it.

### 8.6 Critical correctness tests

These run on every commit; failure blocks merge. Four test categories below — privacy matrix, HMAC canonicalization, upload idempotency, publish lifecycle, and frontend UX — all share the same property: silent regression leaks private content, corrupts data, or breaks the smooth-gallery promise.

#### 8.6.1 Privacy matrix (all surfaces × viewer types)

The previous spec only tested *direct access* (artwork detail and CDN paths). Private artwork can leak through every list/discovery surface — feed, profile, tag, SSR HTML — and through Next.js's caching layers. This matrix is the canonical authoritative test set.

**Setup** (used by every row): user **A** owns artwork **P** (public, tagged `t`) and artwork **Q** (private, tagged `t`). User **B** is a different signed-in user. **Anon** is unauthenticated.

| # | Surface | Anonymous | Owner (A) | Other auth (B) |
|---|---|---|---|---|
| 1 | `GET /artworks` (public feed JSON) | 200; contains P; **must not** contain Q; **no signed `/private/` URLs in payload** | 200; contains P; **must not** contain Q (this is the public feed even for the owner) | 200; contains P; **must not** contain Q |
| 2 | `GET /artworks/:id` where `id=Q` | 404 | 200; payload contains signed `/private/` URL with `sig` + `exp` | 404 (not 403; 404 hides existence) |
| 3 | `GET /artworks/:id` where `id=P` | 200; public CDN URL (no `sig`/`exp`) | 200; public URL | 200; public URL |
| 4 | `GET /users/:slug` (A's profile JSON) | 200; lists P; **must not** list Q; **no signed `/private/` URLs in payload** | 200; lists P **and** Q; Q's images carry signed URLs | 200; lists P; **must not** list Q |
| 5 | `GET /tags/:name` where `name=t` | 200; contains P; **must not** contain Q (tag pages are public-only by design) | 200; contains P; **must not** contain Q | 200; contains P; **must not** contain Q |
| 6 | SSR `/` (home page HTML) | HTML contains no `Q.id`, no `private/`, no `sig=` query string | same | same |
| 7 | SSR `/u/:slug` (HTML, A's profile) | HTML contains no `Q.id`, no `private/`, no `sig=` | HTML contains `Q.id`, signed `/private/` URLs, `sig=` query strings | HTML contains no `Q.id`, no `private/`, no `sig=` |
| 8 | SSR `/tag/:name` (HTML) | HTML contains no `Q.id`, no `private/`, no `sig=` | same | same |
| 9 | SSR `/art/:id` HTML where `id=Q` | renders 404 page; **HTML body must not contain any reference to Q** (no title, no description) | renders 200 page with signed `/private/` URLs | renders 404 page; same scrubbing as anonymous |
| 10 | CDN `cdn.example.com/public/<key>` (direct GET) | 200; `Cache-Control: public, max-age=31536000, immutable` | 200 | 200 |
| 11 | CDN `cdn.example.com/private/<key>` (no `sig`) | 401 | 401 (Worker auth is by signed URL, not cookies) | 401 |
| 12 | CDN `cdn.example.com/private/<key>?sig=valid&exp=future` | 200; `Cache-Control: private, no-store` | 200 | 200 (any holder of a valid signed URL gets through — by design; that's why TTL is 5 min) |
| 13 | CDN `cdn.example.com/cdn-cgi/image/width=800,format=auto,quality=85/private/<key>?sig=valid&exp=future` (transformed) | 200 | 200 | 200 |
| 14 | CDN transformed URL with **no** `sig` | 401 | 401 | 401 |

**Negative-content assertions** (cases 1, 4, 6, 7, 8, 9 anon/B columns): the test must not just check that Q's row is absent. It must:

- Search the JSON/HTML response body for the literal `private/` substring → must not appear.
- Search for any occurrence of `Q.id` (the artwork's UUID) → must not appear.
- Search for `sig=` or `exp=` query parameters → must not appear.

This catches subtle leaks like: API forgot to filter Q from the feed → row appears in JSON; or SSR component rendered Q but with `display:none` CSS → still in HTML; or response includes related-artwork links that don't honor visibility.

**Next.js cache regression test** (specific case 7 anon column): after an *owner* request hits `/u/:slug` (which would populate any naïve cache with owner-visible HTML), make an *anonymous* request to the same URL and re-verify the negative-content assertions. This catches the Next.js Data Cache footgun called out in §7.6.

#### 8.6.2 HMAC canonicalization round-trip (case 15)

15. **End-to-end signed-URL round-trip.** Test code calls the Go `PrivateImageURL("private/<id>/0.jpg")`, then issues the *transformed* request the browser will actually make (`GET /cdn-cgi/image/width=800,format=auto,quality=85/private/<id>/0.jpg?sig=...&exp=...`) against a deployed dev Worker, and asserts a 200. The only test that catches canonicalization drift between API signing and Worker validation. Runs in CI on every commit that touches either side, against a real Worker.

#### 8.6.3 Upload idempotency (cases 16–19)

16. **Retry with the same `client_image_id` is safe.** POST one image with `client_image_id=K`, position=0. POST again with the *same* `client_image_id` and *same* position. Second response returns the *same* `artwork_image.id` as the first. `SELECT count(*) FROM artwork_images WHERE artwork_id=A` returns 1, not 2.
17. **Duplicate position is rejected.** With image at position=0 (`client_image_id=K1`) already inserted, POST a *different* image (`client_image_id=K2`) at position=0. Response is 412 Precondition Failed; DB state unchanged.
18. **Concurrent uploads do not create duplicate positions.** Spawn N=10 parallel POSTs against the same artwork, each with a unique `client_image_id` and the same position=5. Exactly one returns 200; the other 9 return 412. `SELECT count(*) FROM artwork_images WHERE artwork_id=A AND position=5` = 1.
19. **Failed third image leaves first two valid; user retries the third.** POST 5 files; simulate a server panic on the 3rd file mid-processing. Verify rows for files 0, 1 exist (their per-file transactions committed). Re-POST all 5 files with the same `client_image_id`s. Files 0, 1 return their existing rows; files 2, 3, 4 are processed and inserted. Final state = 5 rows, no duplicates.

#### 8.6.4 Publish lifecycle (case 20)

20. **`published_at` semantics (option A).** Create artwork with `visibility='private'`; verify `published_at IS NULL` and the artwork does *not* appear in `GET /artworks`. Flip to `public`; verify `published_at` is now `~= now()` and the artwork appears in the feed. Flip private→public→private→public again; verify `published_at` is unchanged from the first public transition.

#### 8.6.5 Frontend UX correctness (cases 21–24)

These run as Playwright E2E tests against a docker-compose'd full stack on every commit.

21. **Infinite scroll does not duplicate items.** Load `/`, scroll to trigger 5 page fetches (~120 items). Collect all rendered `data-artwork-id` attributes; assert each appears exactly once. Catches cursor-pagination off-by-one bugs and React-key collision bugs.
22. **Masonry reserves dimensions; no layout jump.** Load `/` with network throttled to "Slow 3G." Take a screenshot at 0ms (placeholders only) and at 5000ms (images loaded). Assert the bounding boxes of all visible cards have not moved (Cumulative Layout Shift < 0.05 over the load). Catches missing `width`/`height` on `next/image` and broken aspect-ratio reservation.
23. **Lazy loading only requests images near the viewport.** Load `/` with network logging. Initially, only images within the first 2 viewport heights should have requested image bytes (look for `/cdn-cgi/image/...` requests). Scroll down 3 viewport heights; verify additional image requests now appear. Catches accidental `loading="eager"` or broken intersection-observer.
24. **Flip private + incognito = disappearance.** Owner signs in, posts artwork P (public). Verify P appears at `/u/<slug>` in an incognito window. Owner flips P to private. Within ~30s (allowing CDN purge), reload the incognito window and assert P no longer appears at `/u/<slug>`, `/`, `/tag/<name>`, or `/art/<id>` (the last must show the 404 page with no leaked metadata).

## 9. Testing Strategy

### 9.1 Go (`go test`)

| Layer | What | How |
|---|---|---|
| Repositories | DB queries, constraints, unique violations, indexes | Real Postgres via testcontainers; fresh schema per test |
| Services | Business logic (privacy flip, upload pipeline, feed query) | Real DB + in-memory `Storage` fake for speed |
| HTTP handlers | Routing, auth middleware, status codes, error mapping | `httptest` against the full chain with real DB |
| Auth | OAuth callback, JWT issue/verify, cookie attributes | Mock Google's token endpoint; everything else real |
| Storage impls | `localfs.Store` and `r2.Store` | Real filesystem for localfs; testcontainers MinIO for r2 |
| Critical correctness | All 24 cases in §8.6 (privacy matrix 14, HMAC round-trip 1, upload idempotency 4, publish lifecycle 1, frontend UX 4) | Privacy matrix: `internal/artwork/privacy_matrix_test.go` (Go) + `e2e/privacy_html.spec.ts` (Playwright). Upload: `internal/image/upload_idempotency_test.go`. Publish: `internal/artwork/publish_lifecycle_test.go`. Frontend UX: `e2e/ux.spec.ts`. |

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

- **Cloudflare Worker maintenance burden**: it's a small piece of infra outside the main repos. Mitigation: keep it under 50 lines, test against a deployed dev Worker as part of E2E.
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
- All 24 critical correctness tests in §8.6 pass (privacy matrix across all surfaces and viewer types, HMAC canonicalization round-trip, upload idempotency, publish lifecycle, frontend UX).
- Lighthouse score on the home feed ≥ 90 for Performance with 50 images on screen on a throttled 4G connection.
