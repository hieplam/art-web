# Art Gallery Web App - C4 Architecture

**Date:** 2026-04-30
**Status:** Draft companion architecture doc
**Source spec:** [2026-04-30-art-gallery-design.md](./2026-04-30-art-gallery-design.md)

This document describes the v1 architecture using the first three C4 model layers only:

- **Level 1 - System Context:** who uses the product and which external systems it depends on.
- **Level 2 - Containers:** deployable/runtime parts of the system and how they communicate.
- **Level 3 - Components:** major components inside the Next.js app, Go API, and Cloudflare image gateway.

The C4 code level is intentionally omitted.

## Level 1 - System Context

The Art Gallery Web App lets anonymous visitors browse public artwork and lets authenticated artists manage their own artwork, including private works. Image bytes are served outside the app/API path through a Cloudflare Worker image gateway so the browsing experience stays smooth under many image requests.

```mermaid
flowchart LR
  anonymous["Anonymous visitor"]
  artist["Artist"]
  system["Art Gallery Web App\nBrowse, upload, publish, and protect artwork"]
  google["Google OAuth2\nIdentity provider"]
  cloudflare["Cloudflare platform\nWorker, R2, Images transform, CDN cache"]

  anonymous -->|"Browse public feed, profiles, tags, artwork detail"| system
  artist -->|"Sign in, upload, edit, publish/private artwork"| system
  system -->|"OAuth2 redirect and callback"| google
  system -->|"Stores and serves image objects through private bindings"| cloudflare
```

### External Actors

| Actor | Goal |
|---|---|
| Anonymous visitor | Browse public artwork without signing in, open artwork detail pages, inspect full display-resolution images, visit artist and tag pages. |
| Artist | Sign in with Google, upload multi-image artwork, edit profile/artwork metadata, publish artwork, make artwork private, view their own private works. |
| Google OAuth2 | Authenticates artists and returns profile identity used to create or find a user account. |
| Cloudflare platform | Hosts the image gateway, stores source images in R2, transforms images on demand, and caches public transformed images. |

## Level 2 - Containers

The system is split so HTML/API data and image bytes use different runtime paths. The Go API owns business rules and metadata. The Cloudflare Worker owns image access, transformations, and cache headers.

```mermaid
flowchart LR
  browser["Browser\nHTML, RSC payloads, JSON, img requests"]
  next["Next.js web app\nexample.com\nApp Router, RSC, next/image"]
  api["Go API\napi.example.com\nchi REST API, auth cookie"]
  db[("PostgreSQL\nUsers, artworks, images, tags")]
  worker["Cloudflare Worker image gateway\ncdn.example.com/img/..."]
  r2[("Cloudflare R2\nPrivate bucket\npublic/... and private/...")]
  images["Cloudflare Images binding\nenv.IMAGES\nResize and format conversion"]
  google["Google OAuth2"]

  browser -->|"Page requests"| next
  browser -->|"Incremental JSON fetches"| next
  next -->|"Server-side fetch with Cookie"| api
  browser -->|"Image requests with w/fmt/q\nand sig/exp for private"| worker
  api -->|"SQL"| db
  api -->|"OAuth start/callback"| google
  api -->|"Upload original bytes\nvia Storage interface"| r2
  api -->|"Generate public/signed image URLs"| browser
  worker -->|"R2 binding get(key)"| r2
  worker -->|"Transform source bytes"| images
```

### Container Responsibilities

| Container | Responsibilities | Must Not Do |
|---|---|---|
| Browser | Render gallery UI, request pages/data, request transformed image URLs directly from the Worker. | Store JWTs in localStorage; proxy image bytes through Next.js or Go. |
| Next.js web app | SSR/RSC pages, public feed UI, upload/profile/settings UI, cache-safe rendering rules for private surfaces. | Make authorization decisions independently of the Go API. |
| Go API | OAuth flow, session cookie, ownership checks, artwork metadata, upload validation, privacy changes, signed private image URL generation. | Serve image bytes to browsers. |
| PostgreSQL | Durable metadata for users, artwork, image records, tags, visibility, publish lifecycle. | Store original image bytes. |
| Cloudflare Worker image gateway | Single chokepoint for browser image access, HMAC validation for private paths, R2 reads, image transforms, public/private cache headers. | Trust cookies for image access; serve private image bytes before signature validation. |
| Cloudflare R2 | Private object storage for original JPG/PNG uploads. | Be publicly readable. |
| Cloudflare Images binding | On-demand resize and format conversion from Worker-provided source bytes. | Decide authorization. |

## Level 3 - Components

### Next.js Web App Components

```mermaid
flowchart TB
  routes["Route handlers/pages\n/, /u/[slug], /art/[id], /tag/[name], /upload, /settings"]
  apiClient["API client\nTyped fetch helpers, cookie forwarding, no-store for private surfaces"]
  feed["Feed components\nMasonry, ArtCard, InfiniteFeed"]
  upload["ArtworkUploader\nManifest, files, client_image_id, position, retries"]
  authUi["Auth UI\nLogin link, avatar/profile nav"]
  imageLoader["Image URL loader\nBuilds /img/... URLs with w/fmt/q"]
  cacheRules["Cache policy module\nforce-dynamic/no-store for viewer-specific pages"]

  routes --> apiClient
  routes --> feed
  routes --> upload
  routes --> authUi
  feed --> imageLoader
  upload --> apiClient
  routes --> cacheRules
```

| Component | Responsibility |
|---|---|
| Route handlers/pages | Render public feed, artwork detail, artist profile, tag page, upload, and settings routes. |
| API client | Calls the Go API, forwards cookies during SSR, uses `cache: 'no-store'` for viewer-specific or signed-URL responses. |
| Feed components | Render masonry feed, preserve aspect ratios, show blur placeholders, append pages through intersection observer. |
| ArtworkUploader | Builds upload manifest, assigns per-file `client_image_id` and `position`, retries failed files safely. |
| Image URL loader | Adds transform params (`w`, `fmt`, `q`) to Worker image URLs without signing anything in the browser. |
| Cache policy module | Enforces dynamic/no-store behavior for `/u/[slug]`, `/art/[id]`, `/upload`, and `/settings`. |

### Go API Components

```mermaid
flowchart TB
  router["HTTP router\nchi routes and middleware"]
  auth["Auth module\nProvider registry, OAuth state, JWT cookie"]
  userSvc["User service\nProfile, slug, avatar"]
  artSvc["Artwork service\nFeed, detail, visibility, tags"]
  uploadSvc["Image upload service\nValidate, decode, blurhash, idempotency"]
  storage["Storage interface\nlocalfs/R2 Put, Move, Delete, URL generation"]
  repos["Repositories\npgx queries and transactions"]
  signer["Image URL signer\nHMAC for private canonical paths"]

  router --> auth
  router --> userSvc
  router --> artSvc
  router --> uploadSvc
  userSvc --> repos
  artSvc --> repos
  uploadSvc --> repos
  uploadSvc --> storage
  artSvc --> storage
  artSvc --> signer
```

| Component | Responsibility |
|---|---|
| HTTP router | Maps REST endpoints, applies auth middleware, translates errors to HTTP status codes. |
| Auth module | Handles `/auth/{provider}/start`, `/auth/{provider}/callback`, provider registration, state validation, JWT issue/verify. |
| User service | Creates/upserts users from OAuth profiles, manages display name, slug, and avatar. |
| Artwork service | Applies visibility rules, feed/profile/tag filtering, publish lifecycle, private/public flips. |
| Image upload service | Owns multipart manifest processing, size/type validation, image decode, blurhash, per-file idempotency. |
| Storage interface | Hides local filesystem vs R2, supports original object writes, moves, deletes, and URL construction. |
| Repositories | Execute SQL using real PostgreSQL schema and transactions. |
| Image URL signer | Creates signed private origin URLs using canonical string `v1|/private/<storage_key>|<exp>`. |

### Cloudflare Worker Image Gateway Components

```mermaid
flowchart TB
  requestParser["Request parser\n/img/public/... or /img/private/..."]
  paramSanitizer["Transform param sanitizer\nAllowlisted w, fmt, q"]
  hmac["Private access verifier\nsig/exp, constant-time compare"]
  r2Reader["R2 reader\nGet source bytes by storage_key"]
  transformer["Images binding adapter\nenv.IMAGES transform/output"]
  cachePolicy["Cache policy\npublic immutable, private no-store"]
  response["HTTP response\nImage bytes and headers"]

  requestParser --> paramSanitizer
  requestParser --> hmac
  hmac --> r2Reader
  paramSanitizer --> transformer
  r2Reader --> transformer
  transformer --> cachePolicy
  cachePolicy --> response
```

| Component | Responsibility |
|---|---|
| Request parser | Converts `/img/<storage_key>` into an R2 key and rejects path traversal or unsupported prefixes. |
| Transform param sanitizer | Allows only safe widths/formats/qualities; clamps or rejects unsupported values. |
| Private access verifier | Requires valid `sig` and future `exp` before any R2 read for `/img/private/...`. |
| R2 reader | Reads original source bytes through the private R2 binding. |
| Images binding adapter | Applies resize and output format conversion using `env.IMAGES`. |
| Cache policy | Public images may be cached aggressively; private images return `Cache-Control: private, no-store`. |

## Use Case Diagrams

### UC1 - Anonymous Browses Public Feed

```mermaid
sequenceDiagram
  actor Visitor as Anonymous visitor
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL
  participant Worker as Image Worker
  participant R2 as R2
  participant Images as Images binding

  Visitor->>Web: GET /
  Web->>API: GET /artworks?limit=24
  API->>DB: Query public artworks only
  DB-->>API: Artwork cards with dimensions and public storage keys
  API-->>Web: JSON with public image URLs
  Web-->>Visitor: HTML masonry feed with placeholders
  Visitor->>Worker: GET /img/public/...?...w=640&fmt=auto&q=85
  Worker->>R2: Get public source object
  R2-->>Worker: Original image bytes
  Worker->>Images: Resize/format source bytes
  Images-->>Worker: Optimized image bytes
  Worker-->>Visitor: 200 image, public cache headers
```

### UC2 - Anonymous Opens Artwork Detail

```mermaid
sequenceDiagram
  actor Visitor as Anonymous visitor
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL
  participant Worker as Image Worker

  Visitor->>Web: GET /art/{id}
  Web->>API: GET /artworks/{id}
  API->>DB: Load artwork if public
  alt Artwork is public
    DB-->>API: Artwork, artist name, published_at, tags, images
    API-->>Web: Public detail payload
    Web-->>Visitor: Detail page
    Visitor->>Worker: GET public image URLs
    Worker-->>Visitor: Transformed images
  else Artwork is private or missing
    API-->>Web: 404 without metadata
    Web-->>Visitor: 404 page with no private references
  end
```

### UC3 - Artist Signs In With Google

```mermaid
sequenceDiagram
  actor Artist
  participant Web as Next.js web app
  participant API as Go API
  participant Google as Google OAuth2
  participant DB as PostgreSQL

  Artist->>Web: Click sign in
  Web-->>Artist: Link to /auth/google/start
  Artist->>API: GET /auth/google/start
  API-->>Artist: 302 to Google with state
  Artist->>Google: Approve OAuth consent
  Google-->>API: GET /auth/google/callback?code&state
  API->>Google: Exchange code and fetch profile
  Google-->>API: OAuth profile
  API->>DB: Upsert user by provider + subject
  DB-->>API: User row
  API-->>Artist: Set HttpOnly cookie and redirect to web app
```

### UC4 - Artist Uploads Multi-Image Artwork

```mermaid
sequenceDiagram
  actor Artist
  participant Web as Next.js upload UI
  participant API as Go API
  participant Upload as Upload service
  participant DB as PostgreSQL
  participant R2 as R2

  Artist->>Web: Select images, title, description, tags
  Web->>API: POST /artworks
  API->>DB: Create private draft, published_at NULL
  DB-->>API: Artwork id
  API-->>Web: Draft id
  Web->>Web: Build manifest with client_image_id and position
  Web->>API: POST /artworks/{id}/images multipart
  loop Each file
    API->>Upload: Process manifest entry and file
    Upload->>DB: Check existing client_image_id
    alt Retry already exists
      DB-->>Upload: Existing image row
    else New file
      Upload->>Upload: Validate type/size, decode dimensions, compute blurhash
      Upload->>R2: Put original bytes at private/{artwork_id}/{image_id}.ext
      Upload->>DB: Insert artwork_images row
    end
  end
  API-->>Web: Uploaded image rows
```

### UC5 - Artist Publishes Artwork

```mermaid
sequenceDiagram
  actor Artist
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL
  participant R2 as R2

  Artist->>Web: Set visibility to public
  Web->>API: PATCH /artworks/{id} visibility=public
  API->>DB: Begin transaction and verify owner
  API->>R2: Move private/... objects to public/...
  API->>DB: Update storage_key rows to public/...
  API->>DB: Set visibility='public', published_at=COALESCE(published_at, now())
  API->>DB: Commit
  API-->>Web: Updated artwork
  Web-->>Artist: Artwork appears in public feed/profile/tag surfaces
```

### UC6 - Artist Makes Artwork Private

```mermaid
sequenceDiagram
  actor Artist
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL
  participant R2 as R2
  participant CF as Cloudflare cache

  Artist->>Web: Set visibility to private
  Web->>API: PATCH /artworks/{id} visibility=private
  API->>DB: Begin transaction and verify owner
  API->>R2: Move public/... objects to private/...
  API->>DB: Update storage_key rows to private/...
  API->>DB: Set visibility='private' and keep published_at
  API->>DB: Commit
  API-->>Web: Updated artwork
  API-->>CF: Async purge old public transformed URLs
  Web-->>Artist: Artwork hidden from public feed/profile/tag, still visible to owner
```

### UC7 - Owner Views Private Artwork

```mermaid
sequenceDiagram
  actor Artist
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL
  participant Worker as Image Worker
  participant R2 as R2
  participant Images as Images binding

  Artist->>Web: GET /art/{private_id} with auth cookie
  Web->>API: GET /artworks/{private_id} with Cookie, cache=no-store
  API->>DB: Load artwork and verify owner
  DB-->>API: Private artwork and image rows
  API-->>Web: Detail payload with signed private image URLs
  Web-->>Artist: Dynamic no-store page
  Artist->>Worker: GET /img/private/...?sig=...&exp=...&w=1280
  Worker->>Worker: Verify sig/exp before reading R2
  Worker->>R2: Get private source object
  R2-->>Worker: Original bytes
  Worker->>Images: Transform source bytes
  Images-->>Worker: Optimized bytes
  Worker-->>Artist: 200 image, Cache-Control private no-store
```

### UC8 - Non-Owner Attempts Private Artwork

```mermaid
sequenceDiagram
  actor Viewer as Anonymous or other user
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL
  participant Worker as Image Worker

  Viewer->>Web: GET /art/{private_id}
  Web->>API: GET /artworks/{private_id}
  API->>DB: Load visible artwork for viewer
  DB-->>API: No visible row
  API-->>Web: 404 with no private metadata
  Web-->>Viewer: 404 page
  Viewer->>Worker: GET /img/private/... without valid sig
  Worker-->>Viewer: 401 before R2 read
```

### UC9 - Artist Profile and Tag Browsing

```mermaid
sequenceDiagram
  actor Viewer as Visitor or signed-in viewer
  participant Web as Next.js web app
  participant API as Go API
  participant DB as PostgreSQL

  alt Artist profile
    Viewer->>Web: GET /u/{slug}
    Web->>API: GET /users/{slug}
    API->>DB: Query public works, plus private works only if viewer owns profile
    DB-->>API: Filtered profile payload
    API-->>Web: Profile JSON
    Web-->>Viewer: Profile page
  else Tag page
    Viewer->>Web: GET /tag/{name}
    Web->>API: GET /tags/{name}
    API->>DB: Query public artworks for tag only
    DB-->>API: Public tag results
    API-->>Web: Tag JSON
    Web-->>Viewer: Tag page
  end
```

## Cross-Cutting Rules

### Privacy

- Public list surfaces must filter to `visibility='public' AND published_at IS NOT NULL`.
- Owner-only surfaces must be dynamic and `no-store`.
- Private image URLs must contain short-lived `sig` and `exp`.
- The Worker must validate private signatures before reading R2.
- Private transformed responses must be `Cache-Control: private, no-store`.
- Anonymous and non-owner responses must not include private IDs, private paths, signed URLs, or private metadata.

### Performance

- The API and Next.js never proxy image bytes.
- Image cards always include width/height so the masonry layout reserves space before bytes arrive.
- Public transformed images may be cached aggressively by Cloudflare.
- Private transformed images are not shared-cacheable; privacy takes priority over CDN reuse.
- Infinite scroll appends cursor-paginated JSON and must not duplicate artwork IDs.

### Test Mapping

| Use case | Primary tests |
|---|---|
| UC1 public feed | Privacy matrix case 1, SSR case 6, UX cases 21-23. |
| UC2 detail page | Privacy matrix cases 2, 3, and 9. |
| UC3 sign in | Auth tests for OAuth callback, state validation, cookie attributes. |
| UC4 upload | Upload idempotency cases 16-19. |
| UC5 publish | Publish lifecycle case 20 and feed/profile/tag visibility checks. |
| UC6 private flip | Privacy matrix, CDN purge E2E, incognito disappearance case 24. |
| UC7 owner private view | Privacy matrix cases 2, 7, 12, 13. |
| UC8 non-owner private denial | Privacy matrix cases 2, 9, 11, 14. |
| UC9 profile/tag browse | Privacy matrix cases 4, 5, 7, 8. |
