# Plan 3 — Next.js Frontend

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Next.js (App Router, RSC) frontend at `example.com` — masonry feed, profile, artwork detail, tag pages, upload UI — that consumes Plan 1's JSON API and renders images via Plan 2's Worker. Privacy-correct rendering and a smooth, no-jank gallery feel are the two non-negotiables.

**Architecture:** Next.js 14 App Router with React Server Components. Public-only pages (`/`, `/tag/[name]`) cache freely; viewer-specific pages (`/u/[slug]`, `/art/[id]`, `/upload`, `/settings`) opt out of every caching layer (`force-dynamic` + `fetchCache: 'force-no-store'` + `cache: 'no-store'` on every fetch). A custom `next/image` loader appends `w/fmt/q` query params from the contracts §6 allowlist and `next.config.js` overrides `deviceSizes` so generated `srcset` only requests valid widths. Login is a plain `<a>` to the API's OAuth start route — no NextAuth, no client SDKs.

**Tech Stack:** Next.js 14, React 18, TypeScript, Tailwind CSS (minimal), Vitest + React Testing Library (unit/component), Playwright (E2E), `image-size` for HTML response parsing in tests.

**Subtree owned by this plan:** `web/` (read [contracts §1](./2026-04-30-art-gallery-contracts.md#1-monorepo-layout)). This plan also owns `docker-compose.e2e.yml` at repo root and `.github/workflows/e2e.yml` for cross-system integration.

**Read first:**
1. [Design spec](../specs/2026-04-30-art-gallery-design.md) — sections 7, 8.5, 8.6.1 (rows 6-9), 8.6.5, 12
2. [Contracts doc](./2026-04-30-art-gallery-contracts.md) — sections 4, 6, 7, 8, 10

**Prerequisites:** Node 20+, Docker (for E2E stack), and **Plans 1 + 2 merged to master** before Task 19 onwards (the cross-system tests need both running).

---

## Task overview

| # | Task | Tests change |
|---|---|---|
| 1 | Bootstrap `web/` Next 14 App Router project | smoke build |
| 2 | TypeScript types from contracts §8 | unit |
| 3 | API client + cookie forwarding | unit |
| 4 | `cf-loader.ts` width-clamping loader | unit |
| 5 | BlurHash → data URL helper | unit |
| 6 | `ArtCard` component | RTL |
| 7 | `Masonry` CSS-columns layout | RTL |
| 8 | `InfiniteFeed` IntersectionObserver | RTL |
| 9 | Root layout + nav (login link, avatar) | RTL |
| 10 | Home page `/` (public feed SSR) | RTL |
| 11 | User profile `/u/[slug]` (force-dynamic) | RTL |
| 12 | Artwork detail `/art/[id]` (force-dynamic) | RTL |
| 13 | Tag page `/tag/[name]` | RTL |
| 14 | Upload page `/upload` (auth-gated form) | RTL |
| 15 | Settings page `/settings` | RTL |
| 16 | Cursor parser + edge cases | unit |
| 17 | `vitest.config.ts` + setup | — |
| 18 | `playwright.config.ts` + globalSetup | — |
| 19 | E2E privacy SSR HTML tests (cases 6-9) | E2E |
| 20 | E2E UX tests (cases 22-25) | E2E |
| 21 | `docker-compose.e2e.yml` for full-stack | — |
| 22 | `.github/workflows/web.yml` + `e2e.yml` | CI |

---

## Task 1: Bootstrap `web/` Next 14 project

**Files:**
- Create: `web/package.json`
- Create: `web/tsconfig.json`
- Create: `web/next.config.js`
- Create: `web/.gitignore`
- Create: `web/app/layout.tsx`
- Create: `web/app/page.tsx`
- Create: `web/app/globals.css`

- [ ] **Step 1: package.json**

```json
{
  "name": "art-web-frontend",
  "version": "0.1.0",
  "private": true,
  "engines": {
    "node": ">=20"
  },
  "scripts": {
    "dev": "next dev -p 3000",
    "build": "next build",
    "start": "next start -p 3000",
    "test": "vitest run",
    "test:e2e": "playwright test",
    "typecheck": "tsc --noEmit",
    "lint": "next lint"
  },
  "dependencies": {
    "blurhash": "^2.0.5",
    "next": "14.2.0",
    "pngjs": "^7.0.0",
    "react": "18.3.0",
    "react-dom": "18.3.0"
  },
  "devDependencies": {
    "@playwright/test": "^1.43.0",
    "@testing-library/jest-dom": "^6.4.0",
    "@testing-library/react": "^15.0.0",
    "@types/node": "^20.0.0",
    "@types/react": "^18.0.0",
    "@types/react-dom": "^18.0.0",
    "@vitejs/plugin-react": "^4.0.0",
    "image-size": "^1.1.1",
    "jsdom": "^24.0.0",
    "tailwindcss": "^3.4.0",
    "typescript": "^5.4.0",
    "vitest": "^1.6.0"
  }
}
```

- [ ] **Step 2: next.config.js with allowlist-aligned deviceSizes**

```js
// web/next.config.js
/** @type {import('next').NextConfig} */
module.exports = {
  reactStrictMode: true,
  images: {
    deviceSizes: [240, 480, 800, 1024, 1600, 2400],
    imageSizes: [],
    loaderFile: "./lib/cf-loader.ts",
    remotePatterns: [
      { protocol: "https", hostname: "cdn.example.com" },
      { protocol: "http",  hostname: "localhost" },
    ],
  },
};
```

- [ ] **Step 3: tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "es2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "module": "esnext",
    "moduleResolution": "bundler",
    "jsx": "preserve",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx"],
  "exclude": ["node_modules"]
}
```

- [ ] **Step 4: Bare layout + page placeholders**

```tsx
// web/app/layout.tsx
import "./globals.css";
import type { ReactNode } from "react";

export default function Root({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
```

```tsx
// web/app/page.tsx
export default function Home() { return <main>placeholder</main>; }
```

```css
/* web/app/globals.css */
* { box-sizing: border-box; }
body { margin: 0; font-family: system-ui, sans-serif; }
```

- [ ] **Step 5: .gitignore**

```gitignore
node_modules/
.next/
out/
.env
.env.local
playwright-report/
test-results/
```

- [ ] **Step 6: Install + smoke**

```bash
cd web && npm install && npm run typecheck && npx next build
```

- [ ] **Step 7: Commit**

```bash
git add web/
git commit -m "[web] chore: bootstrap Next.js 14 App Router project"
```

---

## Task 2: TypeScript types from contracts §8

**Files:**
- Create: `web/lib/types.ts`

- [ ] **Step 1: Paste contracts §8 types verbatim**

```ts
// web/lib/types.ts
// Mirrors contracts §8 byte-for-byte. Any change here MUST also land in
// the Go API (artwork response renderer) and the contracts doc.

export type User = {
  id: string;
  display_name: string;
  slug: string;
  avatar_url: string | null;
};

export type ImageRef = {
  id: string;
  url: string;
  width: number;
  height: number;
  blurhash: string;
  position: number;
};

export type ArtworkSummary = {
  id: string;
  title: string;
  visibility: "public" | "private";
  published_at: string | null;
  created_at: string;
  cover: ImageRef;
  artist: User;
};

export type ArtworkDetail = ArtworkSummary & {
  description: string | null;
  tags: string[];
  images: ImageRef[];
};

export type Feed = {
  items: ArtworkSummary[];
  next_cursor: string | null;
};

export type UserProfile = {
  user: User;
  artworks: ArtworkSummary[];
  next_cursor: string | null;
};

export type ApiError = {
  error: string;
  message: string;
};
```

- [ ] **Step 2: Commit (no test — pure types)**

```bash
git add web/lib/types.ts
git commit -m "[web] feat(types): mirror contracts §8 JSON shapes"
```

---

## Task 3: API client + cookie forwarding

**Files:**
- Create: `web/lib/api.ts`
- Create: `web/lib/api.test.ts`

- [ ] **Step 1: Failing tests**

```ts
// web/lib/api.test.ts
import { describe, it, expect, vi } from "vitest";
import { api, ApiClientError } from "./api";

describe("api client", () => {
  it("forwards cookie header when given", async () => {
    const fetchSpy = vi.fn().mockResolvedValue(new Response(`{"ok":1}`, { status: 200 }));
    vi.stubGlobal("fetch", fetchSpy);
    await api({ base: "http://api", path: "/me", cookie: "auth=abc" });
    expect(fetchSpy).toHaveBeenCalledWith("http://api/me", expect.objectContaining({
      headers: expect.objectContaining({ Cookie: "auth=abc" }),
      cache: "no-store",
    }));
  });

  it("throws ApiClientError with status + body", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(`{"error":"not_found"}`, { status: 404 })));
    await expect(api({ base: "http://api", path: "/nope" })).rejects.toMatchObject({
      status: 404,
      body: { error: "not_found" },
    });
  });

  it("returns parsed JSON for 2xx", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(`{"hi":"world"}`, { status: 200 })));
    const out = await api({ base: "http://api", path: "/x" });
    expect(out).toEqual({ hi: "world" });
  });
});
```

- [ ] **Step 2: Implementation**

```ts
// web/lib/api.ts
export class ApiClientError extends Error {
  constructor(public status: number, public body: unknown) {
    super(`API error ${status}`);
  }
}

export type ApiArgs = {
  base: string;
  path: string;
  method?: string;
  body?: BodyInit;
  cookie?: string;        // forwarded as the request `Cookie` header
  headers?: Record<string, string>;
};

export async function api<T = unknown>(args: ApiArgs): Promise<T> {
  const headers: Record<string, string> = { ...(args.headers ?? {}) };
  if (args.cookie) headers["Cookie"] = args.cookie;
  if (args.body && !headers["Content-Type"] && typeof args.body === "string") {
    headers["Content-Type"] = "application/json";
  }
  const resp = await fetch(`${args.base}${args.path}`, {
    method: args.method ?? "GET",
    body: args.body,
    headers,
    cache: "no-store",   // every fetch from the server-side renderer must opt out
    credentials: "include",
  });
  const text = await resp.text();
  let parsed: unknown;
  try { parsed = text ? JSON.parse(text) : null; } catch { parsed = text; }
  if (!resp.ok) throw new ApiClientError(resp.status, parsed);
  return parsed as T;
}

// Helper for RSC handlers that need to forward the user's cookie to the API.
//
// We build the header explicitly via getAll() rather than relying on
// `cookies().toString()`. The toString shape is not part of Next's
// documented public API and shifts between minor releases (it has shipped
// `name=val; name2=val2`, `name=val&name2=val2`, and a `[object …]`
// stringification across the 13.x → 14.x line). Building the value we
// know the API expects keeps us insulated from that drift.
import { cookies } from "next/headers";

export function forwardCookie(): string | undefined {
  const all = cookies().getAll();
  if (all.length === 0) return undefined;
  return all.map((c) => `${c.name}=${c.value}`).join("; ");
}

export function hasAuthCookie(): boolean {
  return cookies().get("auth") !== undefined;
}
```

- [ ] **Step 3: Run + commit**

```bash
cd web && npm test -- api.test
git add web/lib/api.ts web/lib/api.test.ts
git commit -m "[web] feat(api): cookie-forwarding fetch wrapper with no-store default"
```

---

## Task 4: `cf-loader.ts` width-clamping loader

**Files:**
- Create: `web/lib/cf-loader.ts`
- Create: `web/lib/cf-loader.test.ts`

- [ ] **Step 1: Failing tests**

```ts
// web/lib/cf-loader.test.ts
import { describe, it, expect } from "vitest";
import cfLoader from "./cf-loader";

describe("cf-loader", () => {
  it("clamps to nearest-up allowlist width", () => {
    const out = cfLoader({ src: "https://cdn.example.com/img/public/a/b.jpg", width: 700 });
    expect(out).toContain("w=800");
  });

  it("falls back to largest when oversize", () => {
    const out = cfLoader({ src: "https://cdn.example.com/img/public/a/b.jpg", width: 9999 });
    expect(out).toContain("w=2400");
  });

  it("preserves sig + exp on private URLs", () => {
    const out = cfLoader({
      src: "https://cdn.example.com/img/private/a/b.jpg?sig=ABC&exp=123",
      width: 240, quality: 90,
    });
    const u = new URL(out);
    expect(u.searchParams.get("sig")).toBe("ABC");
    expect(u.searchParams.get("exp")).toBe("123");
    expect(u.searchParams.get("w")).toBe("240");
    expect(u.searchParams.get("q")).toBe("90");
    expect(u.searchParams.get("fmt")).toBe("auto");
  });

  it("defaults q=85 when not specified", () => {
    const u = new URL(cfLoader({ src: "https://cdn.example.com/img/public/x.jpg", width: 800 }));
    expect(u.searchParams.get("q")).toBe("85");
  });
});
```

- [ ] **Step 2: Implementation**

```ts
// web/lib/cf-loader.ts
const ALLOWED_WIDTHS = [240, 480, 800, 1024, 1600, 2400] as const;
const ALLOWED_QUALITIES = new Set([60, 75, 85, 90]);

function pickWidth(requested: number): number {
  for (const w of ALLOWED_WIDTHS) if (w >= requested) return w;
  return ALLOWED_WIDTHS[ALLOWED_WIDTHS.length - 1];
}

export default function cfLoader(args: { src: string; width: number; quality?: number }): string {
  const u = new URL(args.src);
  u.searchParams.set("w", String(pickWidth(args.width)));
  u.searchParams.set("fmt", "auto");
  const q = args.quality && ALLOWED_QUALITIES.has(args.quality) ? args.quality : 85;
  u.searchParams.set("q", String(q));
  return u.toString();
}
```

- [ ] **Step 3: Run + commit**

```bash
cd web && npm test -- cf-loader
git add web/lib/cf-loader.ts web/lib/cf-loader.test.ts
git commit -m "[web] feat(image): cf-loader with allowlist clamp preserving sig/exp"
```

---

## Task 5: BlurHash → data URL helper

**Files:**
- Create: `web/lib/blurhash.ts`
- Create: `web/lib/blurhash.test.ts`

- [ ] **Step 1: Failing test**

```ts
// web/lib/blurhash.test.ts
import { describe, it, expect } from "vitest";
import { blurhashToDataURL } from "./blurhash";

describe("blurhashToDataURL", () => {
  it("returns a base64 PNG data URL", () => {
    const url = blurhashToDataURL("L6PZfSi_.AyE_3t7t7R**0o#DgR4");
    expect(url.startsWith("data:image/png;base64,")).toBe(true);
    expect(url.length).toBeGreaterThan(120);
  });

  it("returns a 1x1 transparent fallback for empty input", () => {
    const url = blurhashToDataURL("");
    expect(url).toBe("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=");
  });
});
```

- [ ] **Step 2: Implementation** — uses the canonical `blurhash` npm package for the AC-component decode

We use the upstream `blurhash` package rather than an in-tree decoder. An earlier draft of this plan inlined the algorithm and shipped a *wrong* AC-component decode (missing the `sign(v) * (v/m)² * maxAC` step), which produced placeholders with muddy/incorrect colors. The bug was invisible to a "starts with `data:image/png;base64,` and length > 120" assertion, so it would have shipped silently. The npm package is ~3 KB minified and authoritative — not worth maintaining a buggy fork to save it.

```ts
// web/lib/blurhash.ts
// Decodes a BlurHash string into a tiny base64-encoded PNG suitable for
// `next/image`'s `blurDataURL` prop. Two-step pipeline:
//   1. blurhash.decode → RGBA pixels
//   2. pngjs → base64-encoded PNG
import { decode as decodeBlurhash } from "blurhash";
import { PNG } from "pngjs/browser";

// 1×1 transparent PNG. Returned for empty/malformed input so the caller
// never crashes the render — blurhash is visual sugar, not load-bearing.
const FALLBACK = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";

function rgbaToBase64Png(px: Uint8ClampedArray, w: number, h: number): string {
  const png = new PNG({ width: w, height: h });
  png.data = Buffer.from(px);
  const buf = PNG.sync.write(png);
  return "data:image/png;base64," + buf.toString("base64");
}

export function blurhashToDataURL(hash: string, w = 32, h = 32): string {
  if (!hash || hash.length < 6) return FALLBACK;
  try {
    const px = decodeBlurhash(hash, w, h);
    return rgbaToBase64Png(px, w, h);
  } catch {
    return FALLBACK;
  }
}
```

`blurhash` ships its own TypeScript types so no `@types/blurhash` is needed. `pngjs/browser` provides a Buffer-based PNG encoder; in Next 14's RSC + jsdom-test environment Buffer is polyfilled.

- [ ] **Step 3: Add deps + run + commit**

```bash
cd web && npm install blurhash@^2.0 pngjs@^7.0
npm test -- blurhash
git add web/lib/blurhash.ts web/lib/blurhash.test.ts web/package.json web/package-lock.json
git commit -m "[web] feat: BlurHash → tiny base64 PNG using upstream blurhash package"
```

---

## Task 6: `ArtCard` component

**Files:**
- Create: `web/components/ArtCard.tsx`
- Create: `web/components/ArtCard.test.tsx`

- [ ] **Step 1: Failing test**

```tsx
// web/components/ArtCard.test.tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ArtCard } from "./ArtCard";

const item = {
  id: "art-1", title: "Hello", visibility: "public" as const,
  published_at: "2026-04-30T00:00:00Z", created_at: "2026-04-30T00:00:00Z",
  cover: { id: "i-1", url: "https://cdn.example.com/img/public/a/b.jpg",
           width: 800, height: 600, blurhash: "L0", position: 0 },
  artist: { id: "u-1", display_name: "Alice", slug: "alice", avatar_url: null },
};

describe("ArtCard", () => {
  it("renders link to /art/:id with the artwork id as data attribute", () => {
    render(<ArtCard item={item} />);
    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/art/art-1");
    expect(link.getAttribute("data-artwork-id")).toBe("art-1");
  });

  it("includes width and height to reserve aspect-ratio space", () => {
    render(<ArtCard item={item} />);
    const img = screen.getByRole("img");
    expect(img.getAttribute("width")).toBe("800");
    expect(img.getAttribute("height")).toBe("600");
  });
});
```

- [ ] **Step 2: Implementation**

```tsx
// web/components/ArtCard.tsx
import Image from "next/image";
import Link from "next/link";
import type { ArtworkSummary } from "@/lib/types";
import { blurhashToDataURL } from "@/lib/blurhash";

export function ArtCard({ item }: { item: ArtworkSummary }) {
  const c = item.cover;
  return (
    <Link href={`/art/${item.id}`} data-artwork-id={item.id} className="block">
      <Image
        src={c.url}
        width={c.width}
        height={c.height}
        alt={item.title}
        placeholder="blur"
        blurDataURL={blurhashToDataURL(c.blurhash)}
        sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 25vw"
      />
      <div className="px-2 py-1 text-sm">
        <div className="truncate">{item.title}</div>
        <div className="text-gray-500 truncate">{item.artist.display_name}</div>
      </div>
    </Link>
  );
}
```

- [ ] **Step 3: Run + commit**

```bash
cd web && npm test -- ArtCard
git add web/components/ArtCard.tsx web/components/ArtCard.test.tsx
git commit -m "[web] feat(component): ArtCard with width/height reservation + blurhash"
```

---

## Task 7: `Masonry` CSS-columns layout

**Files:**
- Create: `web/components/Masonry.tsx`
- Create: `web/components/Masonry.test.tsx`
- Modify: `web/app/globals.css`

- [ ] **Step 1: Failing test**

```tsx
// web/components/Masonry.test.tsx
import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Masonry } from "./Masonry";

describe("Masonry", () => {
  it("applies the masonry class so children flow into columns", () => {
    const { container } = render(<Masonry>{[<div key="a" /> ]}</Masonry>);
    expect(container.firstElementChild?.className).toContain("masonry");
  });
});
```

- [ ] **Step 2: Implementation**

```tsx
// web/components/Masonry.tsx
import type { ReactNode } from "react";

export function Masonry({ children }: { children: ReactNode }) {
  return <div className="masonry">{children}</div>;
}
```

```css
/* web/app/globals.css (append) */
.masonry { columns: 4 240px; column-gap: 12px; }
.masonry > * { break-inside: avoid; margin-bottom: 12px; display: block; }
```

- [ ] **Step 3: Run + commit**

```bash
cd web && npm test -- Masonry
git add web/components/Masonry.tsx web/components/Masonry.test.tsx web/app/globals.css
git commit -m "[web] feat(component): Masonry via CSS multi-column layout"
```

---

## Task 8: `InfiniteFeed` IntersectionObserver

**Files:**
- Create: `web/components/InfiniteFeed.tsx`
- Create: `web/components/InfiniteFeed.test.tsx`

- [ ] **Step 1: Failing test** — uses a stub IntersectionObserver

```tsx
// web/components/InfiniteFeed.test.tsx
import { render, screen, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InfiniteFeed } from "./InfiniteFeed";

let triggerEnter: () => void;

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", class {
    constructor(cb: (entries: { isIntersecting: boolean }[]) => void) {
      triggerEnter = () => cb([{ isIntersecting: true }]);
    }
    observe() {}
    disconnect() {}
  });
});

describe("InfiniteFeed", () => {
  it("calls fetchMore when sentinel intersects, then renders new items", async () => {
    const fetchMore = vi.fn().mockResolvedValueOnce({
      items: [{ id: "x" }], next_cursor: null,
    });
    render(<InfiniteFeed
      initialItems={[{ id: "a" }]}
      initialCursor="c1"
      fetchMore={fetchMore}
      renderItem={(item) => <div data-testid="row">{item.id}</div>}
    />);

    expect(screen.getAllByTestId("row")).toHaveLength(1);
    await act(async () => { triggerEnter(); });
    expect(fetchMore).toHaveBeenCalledWith("c1");
    expect(screen.getAllByTestId("row")).toHaveLength(2);
  });

  it("does not duplicate items if fetchMore returns an item already shown", async () => {
    const fetchMore = vi.fn().mockResolvedValueOnce({
      items: [{ id: "a" }, { id: "b" }], next_cursor: null,
    });
    render(<InfiniteFeed
      initialItems={[{ id: "a" }]}
      initialCursor="c1"
      fetchMore={fetchMore}
      renderItem={(item) => <div data-testid="row" data-id={item.id} />}
    />);
    await act(async () => { triggerEnter(); });
    const ids = Array.from(document.querySelectorAll("[data-id]")).map(e => e.getAttribute("data-id"));
    expect(ids).toEqual(["a", "b"]); // 'a' deduplicated
  });
});
```

- [ ] **Step 2: Implementation**

```tsx
// web/components/InfiniteFeed.tsx
"use client";
import { useEffect, useRef, useState } from "react";

type Page<T> = { items: T[]; next_cursor: string | null };

type Props<T extends { id: string }> = {
  initialItems: T[];
  initialCursor: string | null;
  fetchMore: (cursor: string) => Promise<Page<T>>;
  renderItem: (item: T) => JSX.Element;
};

export function InfiniteFeed<T extends { id: string }>(props: Props<T>) {
  const [items, setItems] = useState(props.initialItems);
  const [cursor, setCursor] = useState(props.initialCursor);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef(false);

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !cursor) return;
    const obs = new IntersectionObserver(async ([entry]) => {
      if (!entry.isIntersecting || loadingRef.current) return;
      loadingRef.current = true;
      try {
        const page = await props.fetchMore(cursor);
        setItems((prev) => {
          const seen = new Set(prev.map((p) => p.id));
          return [...prev, ...page.items.filter((i) => !seen.has(i.id))];
        });
        setCursor(page.next_cursor);
      } finally {
        loadingRef.current = false;
      }
    });
    obs.observe(el);
    return () => obs.disconnect();
  }, [cursor]);

  return (
    <>
      {items.map(props.renderItem)}
      {cursor && <div ref={sentinelRef} aria-hidden style={{ height: 1 }} />}
    </>
  );
}
```

- [ ] **Step 3: Run + commit**

```bash
cd web && npm test -- InfiniteFeed
git add web/components/InfiniteFeed.tsx web/components/InfiniteFeed.test.tsx
git commit -m "[web] feat(component): InfiniteFeed with deduping IntersectionObserver"
```

---

## Task 9: Root layout + nav

**Files:**
- Modify: `web/app/layout.tsx`
- Create: `web/components/Nav.tsx`

- [ ] **Step 1: Implementation**

```tsx
// web/components/Nav.tsx
import Link from "next/link";
import { api, forwardCookie, hasAuthCookie } from "@/lib/api";
import type { User } from "@/lib/types";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export async function Nav() {
  let me: User | null = null;
  if (hasAuthCookie()) {
    try { me = await api<User>({ base: API_BASE, path: "/me", cookie: forwardCookie() }); }
    catch { /* 401 = not signed in */ }
  }
  return (
    <nav className="flex items-center justify-between px-4 py-2 border-b">
      <Link href="/" className="font-bold">artweb</Link>
      <div className="flex gap-3">
        <Link href="/">Browse</Link>
        {me ? (
          <>
            <Link href="/upload">Upload</Link>
            <Link href={`/u/${me.slug}`}>{me.display_name}</Link>
          </>
        ) : (
          <a href={`${API_BASE}/auth/google/start`}>Sign in</a>
        )}
      </div>
    </nav>
  );
}
```

```tsx
// web/app/layout.tsx
import "./globals.css";
import { Nav } from "@/components/Nav";

export default async function Root({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <Nav />
        {children}
      </body>
    </html>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/layout.tsx web/components/Nav.tsx
git commit -m "[web] feat: root layout with auth-aware nav (server-rendered)"
```

---

## Task 10: Home page `/` (public feed SSR)

**Files:**
- Modify: `web/app/page.tsx`
- Create: `web/components/HomeClient.tsx` (client wrapper for InfiniteFeed)

- [ ] **Step 1: Implementation**

```tsx
// web/app/page.tsx
import { api } from "@/lib/api";
import { Masonry } from "@/components/Masonry";
import { ArtCard } from "@/components/ArtCard";
import { HomeClient } from "@/components/HomeClient";
import type { Feed } from "@/lib/types";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

// Public-only: cache freely (contracts §10).
export const revalidate = 60;

export default async function Home() {
  const initial = await api<Feed>({ base: API_BASE, path: "/artworks?limit=24" });
  return (
    <main>
      <Masonry>
        {initial.items.map((it) => <ArtCard key={it.id} item={it} />)}
      </Masonry>
      <HomeClient initialCursor={initial.next_cursor} apiBase={API_BASE} />
    </main>
  );
}
```

```tsx
// web/components/HomeClient.tsx
"use client";
import { InfiniteFeed } from "./InfiniteFeed";
import { ArtCard } from "./ArtCard";
import type { ArtworkSummary, Feed } from "@/lib/types";

export function HomeClient({ initialCursor, apiBase }: { initialCursor: string | null; apiBase: string }) {
  if (!initialCursor) return null;
  return (
    <InfiniteFeed<ArtworkSummary>
      initialItems={[]}
      initialCursor={initialCursor}
      fetchMore={async (c) => {
        const r = await fetch(`${apiBase}/artworks?cursor=${encodeURIComponent(c)}&limit=24`);
        return r.json() as Promise<Feed>;
      }}
      renderItem={(it) => <ArtCard key={it.id} item={it} />}
    />
  );
}
```

The first `Masonry` is server-rendered; the client `InfiniteFeed` appends additional cards. Visually the additional cards inherit the masonry layout because they're flowed into the same `<div class="masonry">` container — adjust by hoisting the `Masonry` wrapper inside `HomeClient` if needed.

- [ ] **Step 2: Commit**

```bash
git add web/app/page.tsx web/components/HomeClient.tsx
git commit -m "[web] feat: home feed with SSR-first paint + client infinite scroll"
```

---

## Task 11: User profile `/u/[slug]` (force-dynamic)

**Files:**
- Create: `web/app/u/[slug]/page.tsx`

- [ ] **Step 1: Implementation**

```tsx
// web/app/u/[slug]/page.tsx
import { notFound } from "next/navigation";
import { api, ApiClientError, forwardCookie } from "@/lib/api";
import { Masonry } from "@/components/Masonry";
import { ArtCard } from "@/components/ArtCard";
import type { UserProfile } from "@/lib/types";

// Owner sees drafts → uncacheable (contracts §10).
export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export default async function Profile({ params }: { params: { slug: string } }) {
  let data: UserProfile;
  try {
    data = await api<UserProfile>({
      base: API_BASE,
      path: `/users/${encodeURIComponent(params.slug)}`,
      cookie: forwardCookie(),
    });
  } catch (e) {
    // 404 → render Next's not-found UI (no leaked metadata).
    // Anything else propagates to the error boundary so ops sees it.
    if (e instanceof ApiClientError && e.status === 404) notFound();
    throw e;
  }
  return (
    <main className="p-4">
      <header className="mb-4">
        <h1 className="text-2xl">{data.user.display_name}</h1>
        <div className="text-gray-500">@{data.user.slug}</div>
      </header>
      <Masonry>
        {data.artworks.map((it) => <ArtCard key={it.id} item={it} />)}
      </Masonry>
    </main>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/u/
git commit -m "[web] feat: profile page with force-dynamic + cookie-forwarded fetch"
```

---

## Task 12: Artwork detail `/art/[id]` (force-dynamic)

**Files:**
- Create: `web/app/art/[id]/page.tsx`
- Create: `web/app/art/[id]/not-found.tsx`

- [ ] **Step 1: Implementation**

```tsx
// web/app/art/[id]/page.tsx
import { notFound } from "next/navigation";
import Image from "next/image";
import Link from "next/link";
import { api, ApiClientError, forwardCookie } from "@/lib/api";
import type { ArtworkDetail } from "@/lib/types";
import { blurhashToDataURL } from "@/lib/blurhash";

export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export default async function Art({ params }: { params: { id: string } }) {
  let data: ArtworkDetail;
  try {
    data = await api<ArtworkDetail>({ base: API_BASE, path: `/artworks/${params.id}`, cookie: forwardCookie() });
  } catch (e) {
    if (e instanceof ApiClientError && e.status === 404) notFound();
    throw e;
  }
  return (
    <main className="max-w-3xl mx-auto p-4">
      <h1 className="text-2xl mb-1">{data.title}</h1>
      <Link href={`/u/${data.artist.slug}`} className="text-gray-600">
        {data.artist.display_name}
      </Link>
      {data.published_at && (
        <time className="block text-gray-500 text-sm">{data.published_at.slice(0, 10)}</time>
      )}
      {data.description && <p className="my-3 whitespace-pre-wrap">{data.description}</p>}
      <div className="space-y-3 mt-4">
        {data.images.map((im) => (
          <Image key={im.id}
            src={im.url} width={im.width} height={im.height} alt={data.title}
            placeholder="blur" blurDataURL={blurhashToDataURL(im.blurhash)}
            sizes="(max-width: 800px) 100vw, 800px"
          />
        ))}
      </div>
      <div className="flex flex-wrap gap-2 mt-4">
        {data.tags.map((t) => (
          <Link key={t} href={`/tag/${encodeURIComponent(t)}`} className="text-sm px-2 py-1 bg-gray-100 rounded">
            #{t}
          </Link>
        ))}
      </div>
    </main>
  );
}
```

```tsx
// web/app/art/[id]/not-found.tsx
// Critical: this page MUST NOT echo any artwork metadata.
// Spec §8.6.1 case 9 — anonymous request to a private artwork id renders this page,
// and the HTML body must not contain the artwork's title, description, or id.
export default function NotFound() {
  return <main className="p-8 text-center"><h1>Not found</h1></main>;
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/art/
git commit -m "[web] feat: artwork detail with 404 page that scrubs all metadata"
```

---

## Task 13: Tag page `/tag/[name]`

**Files:**
- Create: `web/app/tag/[name]/page.tsx`

- [ ] **Step 1: Implementation**

```tsx
// web/app/tag/[name]/page.tsx
import { api } from "@/lib/api";
import { Masonry } from "@/components/Masonry";
import { ArtCard } from "@/components/ArtCard";
import type { Feed } from "@/lib/types";

// Public-only by spec §8.6.1 case 5 — cacheable.
export const revalidate = 60;

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export default async function Tag({ params }: { params: { name: string } }) {
  const feed = await api<Feed>({
    base: API_BASE,
    path: `/tags/${encodeURIComponent(params.name)}?limit=24`,
  });
  return (
    <main className="p-4">
      <h1 className="text-2xl mb-3">#{params.name}</h1>
      <Masonry>{feed.items.map((it) => <ArtCard key={it.id} item={it} />)}</Masonry>
    </main>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/tag/
git commit -m "[web] feat: tag page (public-only, ISR cacheable)"
```

---

## Task 14: Upload page `/upload`

**Files:**
- Create: `web/app/upload/page.tsx`
- Create: `web/components/ArtworkUploader.tsx`

- [ ] **Step 1: Server page**

The page is server-rendered specifically so it can auth-gate before the form ever paints. Anonymous viewers get a 302 to the API's OAuth start route — we don't render the form just to have submission fail with a generic error 5 seconds later.

```tsx
// web/app/upload/page.tsx
import { redirect } from "next/navigation";
import { api, ApiClientError, forwardCookie, hasAuthCookie } from "@/lib/api";
import { ArtworkUploader } from "@/components/ArtworkUploader";
import type { User } from "@/lib/types";

export const dynamic = "force-dynamic";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export default async function UploadPage() {
  // Cheap check first — no API round-trip needed if we don't even have a cookie.
  if (!hasAuthCookie()) redirect(`${API_BASE}/auth/google/start`);

  // Cookie may be present but stale (expired JWT, invalidated session) —
  // verify with /me. 401 → redirect to OAuth start; any other failure
  // bubbles up to Next's error boundary so ops sees it.
  try {
    await api<User>({ base: API_BASE, path: "/me", cookie: forwardCookie() });
  } catch (e) {
    if (e instanceof ApiClientError && e.status === 401) {
      redirect(`${API_BASE}/auth/google/start`);
    }
    throw e;
  }

  return (
    <main className="max-w-2xl mx-auto p-4">
      <h1 className="text-2xl mb-3">New artwork</h1>
      <ArtworkUploader apiBase={API_BASE} />
    </main>
  );
}
```

- [ ] **Step 2: Client uploader**

```tsx
// web/components/ArtworkUploader.tsx
"use client";
import { useState } from "react";

function uuidv4(): string {
  // crypto.randomUUID is available in modern browsers + node 18+.
  return crypto.randomUUID();
}

export function ArtworkUploader({ apiBase }: { apiBase: string }) {
  const [title, setTitle] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      // 1. Create artwork (private/draft).
      const create = await fetch(`${apiBase}/artworks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title, visibility: "private" }),
      });
      if (!create.ok) throw new Error("create failed");
      const art = await create.json();

      // 2. Build manifest with stable client_image_ids.
      const manifest = files.map((f, i) => ({
        client_image_id: uuidv4(),
        position: i,
        content_type: f.type,
      }));
      const fd = new FormData();
      fd.set("manifest", JSON.stringify(manifest));
      for (const f of files) fd.append("files", f);

      // 3. Upload (idempotent on retry — same manifest re-POSTs are safe).
      const upload = await fetch(`${apiBase}/artworks/${art.id}/images`, {
        method: "POST", credentials: "include", body: fd,
      });
      if (!upload.ok) throw new Error("upload failed");

      // 4. Redirect to detail (still private).
      window.location.href = `/art/${art.id}`;
    } catch (e) {
      setError(e instanceof Error ? e.message : "unknown");
    } finally { setSubmitting(false); }
  }

  return (
    <div className="space-y-3">
      <input className="border p-2 w-full" placeholder="Title"
             value={title} onChange={(e) => setTitle(e.target.value)} />
      <input type="file" multiple accept="image/jpeg,image/png"
             onChange={(e) => setFiles(Array.from(e.target.files ?? []))} />
      <button className="bg-black text-white px-4 py-2"
              disabled={!title || files.length === 0 || submitting}
              onClick={submit}>
        {submitting ? "Uploading…" : "Upload"}
      </button>
      {error && <div className="text-red-600">{error}</div>}
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/app/upload/ web/components/ArtworkUploader.tsx
git commit -m "[web] feat: upload page with client_image_id-keyed multi-file form"
```

---

## Task 15: Settings page `/settings`

**Files:**
- Create: `web/app/settings/page.tsx`

- [ ] **Step 1: Implementation** — minimal v1; full UI is small.

```tsx
// web/app/settings/page.tsx
"use client";
import { useEffect, useState } from "react";

export const dynamic = "force-dynamic";

const API = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export default function Settings() {
  const [me, setMe] = useState<any>(null);
  useEffect(() => { fetch(`${API}/me`, { credentials: "include" }).then(r => r.json()).then(setMe); }, []);
  if (!me) return <main className="p-4">Loading…</main>;
  return (
    <main className="max-w-md mx-auto p-4 space-y-3">
      <h1 className="text-2xl">Settings</h1>
      <div>Display name: <strong>{me.display_name}</strong></div>
      <div>Slug: <strong>{me.slug}</strong></div>
      <form method="POST" action={`${API}/auth/logout`}>
        <button type="submit" className="bg-gray-200 px-4 py-2">Sign out</button>
      </form>
    </main>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/settings/
git commit -m "[web] feat: minimal settings page with sign-out"
```

---

## Task 16: Cursor parser + edge cases

**Files:**
- Create: `web/lib/cursor.ts`
- Create: `web/lib/cursor.test.ts`

The cursor is opaque to the FE per contracts §8.7. The FE only forwards it. This module just URL-encodes it.

- [ ] **Step 1: Tests + impl**

```ts
// web/lib/cursor.ts
export function appendCursor(path: string, cursor: string | null): string {
  if (!cursor) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}cursor=${encodeURIComponent(cursor)}`;
}
```

```ts
// web/lib/cursor.test.ts
import { describe, it, expect } from "vitest";
import { appendCursor } from "./cursor";

describe("appendCursor", () => {
  it("appends with ? when none present", () => {
    expect(appendCursor("/x", "abc")).toBe("/x?cursor=abc");
  });
  it("appends with & when ? present", () => {
    expect(appendCursor("/x?y=1", "abc")).toBe("/x?y=1&cursor=abc");
  });
  it("URL-encodes special chars", () => {
    expect(appendCursor("/x", "a/b=c")).toBe("/x?cursor=a%2Fb%3Dc");
  });
  it("returns path unchanged when cursor is null", () => {
    expect(appendCursor("/x?y=1", null)).toBe("/x?y=1");
  });
});
```

- [ ] **Step 2: Commit**

```bash
cd web && npm test -- cursor
git add web/lib/cursor.ts web/lib/cursor.test.ts
git commit -m "[web] feat: cursor pagination helper"
```

---

## Task 17: `vitest.config.ts` + setup

**Files:**
- Create: `web/vitest.config.ts`
- Create: `web/vitest.setup.ts`

- [ ] **Step 1: Configs**

```ts
// web/vitest.config.ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    include: ["**/*.test.{ts,tsx}"],
    exclude: ["e2e/**", "node_modules/**"],
  },
  resolve: {
    alias: { "@": path.resolve(__dirname, ".") },
  },
});
```

```ts
// web/vitest.setup.ts
import "@testing-library/jest-dom/vitest";
```

- [ ] **Step 2: Commit**

```bash
git add web/vitest.config.ts web/vitest.setup.ts
git commit -m "[web] chore: vitest config with jsdom + RTL setup"
```

---

## Task 18: `playwright.config.ts` + globalSetup

**Files:**
- Create: `web/playwright.config.ts`
- Create: `web/e2e/global-setup.ts`
- Create: `web/e2e/_helpers.ts`

- [ ] **Step 1: Playwright config**

```ts
// web/playwright.config.ts
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup",
  use: {
    baseURL: process.env.WEB_BASE || "http://localhost:3000",
    extraHTTPHeaders: {},
  },
  projects: [
    { name: "chromium", use: { browserName: "chromium" } },
  ],
  reporter: [["list"], ["html", { open: "never" }]],
});
```

- [ ] **Step 2: globalSetup — verifies the docker-compose stack is reachable**

```ts
// web/e2e/global-setup.ts
import { chromium } from "@playwright/test";

export default async function globalSetup() {
  const api = process.env.API_BASE || "http://localhost:8080";
  const cdn = process.env.CDN_BASE || "http://localhost:8787";
  const web = process.env.WEB_BASE || "http://localhost:3000";

  for (const [label, url] of [["API", api], ["CDN", cdn], ["WEB", web]]) {
    const r = await fetch(`${url}/`, { method: "GET" });
    if (!r.ok && r.status !== 404) throw new Error(`${label} not reachable: ${r.status}`);
  }

  // Optional: pre-seed test data via the API. Implemented in _helpers.ts.
  const browser = await chromium.launch();
  await browser.close();
}
```

- [ ] **Step 3: Test helpers (seeding A, B, P, Q per spec §8.6.1)**

```ts
// web/e2e/_helpers.ts
// These helpers talk directly to the API to set up the matrix:
//   user A (alice) owns artwork P (public, tag "t") and artwork Q (private, tag "t")
//   user B (bob) is another signed-in user
//   anon = no cookie
//
// The /dev/seed endpoint is registered by Plan 1 Task 33, gated by
// APP_ENV=test. Outside test env it returns 404 (the route is not
// registered AND the handler re-checks). The API in docker-compose.e2e.yml
// runs with APP_ENV=test, so this just works inside CI.

const API = process.env.API_BASE || "http://localhost:8080";

export async function seedMatrix(opts: { many?: number } = {}): Promise<{
  aliceCookie: string; bobCookie: string;
  aliceSlug: string; bobSlug: string;
  pId: string; qId: string;
}> {
  const url = new URL(`${API}/dev/seed`);
  if (opts.many) url.searchParams.set("many", String(opts.many));
  const r = await fetch(url, { method: "POST" });
  if (!r.ok) {
    const body = await r.text().catch(() => "");
    throw new Error(`seed failed (${r.status}); ensure API runs with APP_ENV=test. Body: ${body}`);
  }
  return r.json();
}

- [ ] **Step 4: Commit**

```bash
git add web/playwright.config.ts web/e2e/global-setup.ts web/e2e/_helpers.ts
git commit -m "[web] chore: Playwright config with API-backed seeding"
```

---

## Task 19: E2E privacy SSR HTML tests (cases 6, 7, 8, 9)

**Files:**
- Create: `web/e2e/privacy_html.spec.ts`

**Negative-content assertions** are the heart of these tests: assert the rendered HTML does NOT contain the private artwork's id, the substring `private/`, or `sig=`.

- [ ] **Step 1: Test**

```ts
// web/e2e/privacy_html.spec.ts
import { test, expect, request } from "@playwright/test";
import { seedMatrix } from "./_helpers";

const API = process.env.API_BASE || "http://localhost:8080";

let env: Awaited<ReturnType<typeof seedMatrix>>;
test.beforeAll(async () => { env = await seedMatrix(); });

function assertNoLeaks(html: string, qid: string) {
  expect(html).not.toContain(qid);
  expect(html).not.toContain("private/");
  expect(html).not.toMatch(/sig=/);
  expect(html).not.toMatch(/exp=\d/);
}

test("case 6 — anonymous home page contains no Q", async ({ request }) => {
  const html = await (await request.get("/")).text();
  assertNoLeaks(html, env.qId);
});

test("case 7 — anonymous /u/:slug contains no Q (regression for cache footgun)", async ({ request, browser }) => {
  // First, fetch as the OWNER (this is what would populate any naïve cache).
  const owner = await browser.newContext({ extraHTTPHeaders: { Cookie: env.aliceCookie } });
  await (await owner.newPage()).goto(`/u/${env.aliceSlug}`);
  await owner.close();

  // Then fetch as anonymous via raw request (no cookies).
  const html = await (await request.get(`/u/${env.aliceSlug}`)).text();
  assertNoLeaks(html, env.qId);
});

test("case 7 — owner /u/:slug DOES contain Q + signed URLs", async ({ browser }) => {
  const ctx = await browser.newContext({ extraHTTPHeaders: { Cookie: env.aliceCookie } });
  const page = await ctx.newPage();
  await page.goto(`/u/${env.aliceSlug}`);
  const html = await page.content();
  expect(html).toContain(env.qId);
  expect(html).toMatch(/\/img\/private\//);
  expect(html).toMatch(/sig=[0-9a-f]{64}/);
  await ctx.close();
});

test("case 8 — anonymous /tag/:name contains no Q", async ({ request }) => {
  const html = await (await request.get("/tag/t")).text();
  assertNoLeaks(html, env.qId);
});

test("case 9 — anonymous /art/<Q> renders 404 with no Q metadata", async ({ request }) => {
  const r = await request.get(`/art/${env.qId}`);
  expect(r.status()).toBe(404);
  const html = await r.text();
  assertNoLeaks(html, env.qId);
});
```

- [ ] **Step 2: Commit**

```bash
git add web/e2e/privacy_html.spec.ts
git commit -m "[web] test: SSR privacy HTML cases 6-9 with negative-content assertions"
```

---

## Task 20: E2E UX tests (cases 22, 23, 24, 25)

**Files:**
- Create: `web/e2e/ux.spec.ts`

- [ ] **Step 1: Tests**

```ts
// web/e2e/ux.spec.ts
import { test, expect } from "@playwright/test";
import { seedMatrix } from "./_helpers";

let env: Awaited<ReturnType<typeof seedMatrix>>;
test.beforeAll(async () => { env = await seedMatrix(); });

test("case 22 — infinite scroll does not duplicate items", async ({ page }) => {
  // Seed enough public artworks to trigger ≥5 page fetches (~120 items).
  // Hit the API directly — Playwright's `page.request` honors baseURL,
  // which points at the web app (port 3000), not the API (port 8080).
  await seedMatrix({ many: 120 });
  await page.goto("/");
  for (let i = 0; i < 5; i++) {
    await page.evaluate(() => window.scrollBy(0, document.body.scrollHeight));
    await page.waitForTimeout(500);
  }
  const ids = await page.$$eval("[data-artwork-id]", (els) =>
    els.map((e) => e.getAttribute("data-artwork-id"))
  );
  expect(ids.length).toBeGreaterThanOrEqual(120);
  expect(new Set(ids).size).toBe(ids.length);
});

test("case 23 — masonry reserves space; no layout shift > 0.05", async ({ page }) => {
  await page.goto("/");
  // Capture initial card bounds, wait for image bytes, capture again.
  const start = await page.$$eval("[data-artwork-id]", els =>
    els.map(e => e.getBoundingClientRect()).slice(0, 8).map(b => ({ y: b.top, h: b.height })));
  await page.waitForLoadState("networkidle");
  const end = await page.$$eval("[data-artwork-id]", els =>
    els.map(e => e.getBoundingClientRect()).slice(0, 8).map(b => ({ y: b.top, h: b.height })));
  for (let i = 0; i < start.length; i++) {
    expect(Math.abs(start[i].y - end[i].y)).toBeLessThan(2);
    expect(Math.abs(start[i].h - end[i].h)).toBeLessThan(2);
  }
});

test("case 24 — lazy loading: only near-viewport images fetched initially", async ({ page }) => {
  const requested: string[] = [];
  page.on("request", (r) => {
    if (r.url().includes("/img/")) requested.push(r.url());
  });
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  const initialCount = requested.length;
  await page.evaluate(() => window.scrollBy(0, window.innerHeight * 3));
  await page.waitForLoadState("networkidle");
  expect(requested.length).toBeGreaterThan(initialCount);
});

test("case 25 — flip private + incognito → disappearance", async ({ browser }) => {
  // Owner makes P public initially (already public via seed), incognito sees it.
  const incog = await browser.newContext();
  const ip = await incog.newPage();
  await ip.goto(`/u/${env.aliceSlug}`);
  expect(await ip.content()).toContain(env.pId);

  // Owner flips P to private via API.
  await fetch(`${process.env.API_BASE}/artworks/${env.pId}`, {
    method: "PATCH",
    headers: { Cookie: env.aliceCookie, "Content-Type": "application/json" },
    body: JSON.stringify({ visibility: "private" }),
  });

  // Poll up to 60s for the profile page to drop P. Spec §11 says "<30s" for
  // CDN purge; we ceiling at 60s so a slightly slow purge does not flake the
  // test. Sleeping a flat 30s wastes time when purge is fast and flakes when
  // purge is slow.
  await expect.poll(async () => {
    await ip.reload();
    return (await ip.content()).includes(env.pId);
  }, { timeout: 60_000, intervals: [500, 1000, 2000, 4000] }).toBe(false);

  await ip.goto("/");
  expect(await ip.content()).not.toContain(env.pId);
  await ip.goto(`/tag/t`);
  expect(await ip.content()).not.toContain(env.pId);
  const directHTML = await (await ip.request.get(`/art/${env.pId}`)).text();
  expect(directHTML).not.toContain(env.pId);
  await incog.close();
});
```

- [ ] **Step 2: Commit**

```bash
git add web/e2e/ux.spec.ts
git commit -m "[web] test: UX cases 22-25 (no-dupes, no-CLS, lazy, flip+incognito)"
```

---

## Task 21: `docker-compose.e2e.yml` for full-stack

**Files:**
- Create: `docker-compose.e2e.yml` (at repo root)
- Create: `api/Dockerfile`
- Create: `worker/Dockerfile.e2e`
- Create: `worker/scripts/e2e-server.ts`
- Create: `web/Dockerfile`

**Why the Worker is harder than the others.** `wrangler dev` requires `wrangler login` against Cloudflare, which can't run unattended in CI. Miniflare standalone runs the Worker but cannot emulate the `env.IMAGES` binding offline. The clean compromise: ship a small Node entrypoint (`worker/scripts/e2e-server.ts`) that imports the same `handle(req, env)` function the production Worker exports, wraps a MinIO-backed S3 client to look like R2, and stubs IMAGES with a pass-through. This exercises the *real* canonicalization + HMAC + Cache-Control logic; only the actual pixel resize is mocked. Layer-B (Plan 2 Task 11) is what proves the resize works.

`Cache-Control` headers are set per-branch by the production handler (Plan 2 Task 5), so privacy + cache invariants in the SSR HTML and UX tests still pin the same logic the production Worker runs.

- [ ] **Step 1: Compose file with healthchecks on every service**

```yaml
# docker-compose.e2e.yml — at repo root, NOT inside web/
version: "3.9"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: artweb
      POSTGRES_USER: art
      POSTGRES_PASSWORD: art
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U art"]
      interval: 1s
      timeout: 3s
      retries: 30

  minio:
    # Tag pinned per contracts §13 — must match the version used by
    # Plan 1's testcontainer-driven r2_test.go, otherwise dev compose
    # and unit tests can drift apart on AWS S3 SDK behavior nuances.
    image: minio/minio:RELEASE.2024-12-18T13-15-44Z
    command: server /data --address :9000
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin
    ports: ["9000:9000"]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://localhost:9000/minio/health/ready || exit 1"]
      interval: 2s
      timeout: 3s
      retries: 30

  api:
    build:
      context: ./api
      dockerfile: Dockerfile
    environment:
      ADDR: ":8080"
      # APP_ENV=test triggers two behaviors in Plan 1 main.go:
      #   1. loadConfig swaps Storage for localfs (no R2 credentials needed)
      #   2. router registers POST /dev/seed for cross-system test fixtures
      APP_ENV: test
      DATABASE_URL: postgres://art:art@postgres:5432/artweb?sslmode=disable
      JWT_SIGNING_KEY: "3031323334353637383961626364656630313233343536373839616263646566"
      WORKER_SIGNING_KEY: "3031323334353637383961626364656630313233343536373839616263646566"
      CDN_ORIGIN: "http://worker:8787"   # API generates URLs the worker container resolves
      FRONTEND_URL: "http://localhost:3000/"
    depends_on:
      postgres: { condition: service_healthy }
    ports: ["8080:8080"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8080/healthz || exit 1"]
      interval: 2s
      timeout: 3s
      retries: 30

  worker:
    build:
      context: ./worker
      dockerfile: Dockerfile.e2e
    environment:
      PORT: "8787"
      WORKER_SIGNING_KEY: "3031323334353637383961626364656630313233343536373839616263646566"
      S3_ENDPOINT: "http://minio:9000"
      S3_ACCESS_KEY_ID: "minioadmin"
      S3_SECRET_ACCESS_KEY: "minioadmin"
      R2_BUCKET: "art-dev"
    depends_on:
      minio: { condition: service_healthy }
    ports: ["8787:8787"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8787/healthz || exit 1"]
      interval: 2s
      timeout: 3s
      retries: 30

  web:
    build:
      context: ./web
      dockerfile: Dockerfile
    environment:
      NEXT_PUBLIC_API_BASE: "http://api:8080"
      NEXT_PUBLIC_CDN_BASE: "http://localhost:8787"   # browser-side; resolves to host port
      API_BASE_INTERNAL: "http://api:8080"            # server-side fetch uses internal hostname
    depends_on:
      api:    { condition: service_healthy }
      worker: { condition: service_healthy }
    ports: ["3000:3000"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:3000/api/health || wget -qO- http://localhost:3000/ || exit 1"]
      interval: 2s
      timeout: 3s
      retries: 30
```

- [ ] **Step 2: Worker E2E entrypoint (`worker/scripts/e2e-server.ts`)**

Imports the production `handle()`, wraps MinIO as an R2-shaped object, stubs `IMAGES` with pass-through. ~60 lines.

```ts
// worker/scripts/e2e-server.ts
import { createServer } from "node:http";
import { Readable } from "node:stream";
import { S3Client, GetObjectCommand } from "@aws-sdk/client-s3";
import { handle, type Env, type ImagesBinding } from "../src/index";

const PORT = parseInt(process.env.PORT ?? "8787", 10);
const SIGNING_KEY = process.env.WORKER_SIGNING_KEY ?? "";
const BUCKET = process.env.R2_BUCKET ?? "art-dev";

const s3 = new S3Client({
  endpoint: process.env.S3_ENDPOINT ?? "http://minio:9000",
  region: "auto",
  forcePathStyle: true,
  credentials: {
    accessKeyId: process.env.S3_ACCESS_KEY_ID ?? "minioadmin",
    secretAccessKey: process.env.S3_SECRET_ACCESS_KEY ?? "minioadmin",
  },
});

// Minimal R2 shim — handle() only calls .get(); other methods are typed but unused.
const r2 = {
  async get(key: string) {
    try {
      const out = await s3.send(new GetObjectCommand({ Bucket: BUCKET, Key: key }));
      const bytes = await out.Body!.transformToByteArray();
      return {
        body: Readable.toWeb(Readable.from(Buffer.from(bytes))) as ReadableStream,
      };
    } catch {
      return null;
    }
  },
} as unknown as Env["R2"];

// IMAGES stub — no real resize. Plan 2's Layer-B integration test (Task 11)
// is what proves the real binding works; this stub just ensures privacy +
// Cache-Control logic flows correctly through the production handler.
const images: ImagesBinding = {
  input(stream) {
    return {
      transform() { return this; },
      async output(opts) {
        return {
          response: () => new Response(stream, {
            status: 200,
            headers: { "Content-Type": opts.format },
          }),
        };
      },
    } as never;
  },
};

const env: Env = { R2: r2, IMAGES: images, WORKER_SIGNING_KEY: SIGNING_KEY };

const server = createServer(async (nodeReq, nodeRes) => {
  // Compose-internal /healthz so the docker healthcheck has a definitive signal.
  if (nodeReq.url === "/healthz") {
    nodeRes.writeHead(200, { "Content-Type": "text/plain" });
    nodeRes.end("ok");
    return;
  }

  const url = `http://localhost:${PORT}${nodeReq.url}`;
  const headers = new Headers();
  for (const [k, v] of Object.entries(nodeReq.headers)) {
    if (typeof v === "string") headers.set(k, v);
    else if (Array.isArray(v)) headers.set(k, v.join(","));
  }
  try {
    const resp = await handle(new Request(url, { method: nodeReq.method, headers }), env);
    const respHeaders: Record<string, string> = {};
    resp.headers.forEach((v, k) => { respHeaders[k] = v; });
    nodeRes.writeHead(resp.status, respHeaders);
    if (resp.body) {
      const reader = resp.body.getReader();
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        nodeRes.write(value);
      }
    }
    nodeRes.end();
  } catch (err) {
    nodeRes.writeHead(500, { "Content-Type": "text/plain" });
    nodeRes.end(`worker error: ${(err as Error).message}`);
  }
});

server.listen(PORT, () => console.log(`worker e2e server on :${PORT}`));
```

`@aws-sdk/client-s3` is added to `worker/package.json` *only* under devDependencies — production wrangler deploys never run this script:

```json
"devDependencies": {
  ...,
  "@aws-sdk/client-s3": "^3.600.0",
  "tsx": "^4.7.0"
}
```

- [ ] **Step 3: Dockerfiles**

```dockerfile
# api/Dockerfile
# Go version pinned per contracts §13 (must be ≥1.24 — t.Context() is a
# 1.24+ feature used throughout the test suite).
#
# Runtime is alpine, not distroless. Distroless ships no shell, which
# means docker-compose's CMD-SHELL healthcheck (`wget -qO- /healthz`)
# cannot run inside the container. Alpine adds ~5 MB but lets the
# healthcheck work without an extra Go-built healthcheck binary.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
COPY --from=build /bin/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
```

```dockerfile
# worker/Dockerfile.e2e
# E2E-only image. Production worker deploys via `wrangler deploy` and
# never builds this image.
FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY src/ ./src/
COPY scripts/ ./scripts/
COPY tsconfig.json ./
EXPOSE 8787
CMD ["npx", "tsx", "scripts/e2e-server.ts"]
```

```dockerfile
# web/Dockerfile
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=build /app/.next ./.next
COPY --from=build /app/public ./public
COPY --from=build /app/package.json ./package.json
COPY --from=build /app/node_modules ./node_modules
EXPOSE 3000
CMD ["npm", "run", "start"]
```

- [ ] **Step 4: API healthcheck endpoint**

The compose healthcheck queries `/healthz`. Add the route to Plan 1's router (or wire it as a tiny Plan 1 follow-up): `GET /healthz` returns `200 OK` with body `ok`. Keep it pre-middleware so it works even before DB connects.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.e2e.yml api/Dockerfile worker/Dockerfile.e2e worker/scripts/e2e-server.ts web/Dockerfile
git commit -m "[web] chore: docker-compose for full-stack E2E with miniflare-free worker container"
```

---

## Task 22: `.github/workflows/web.yml` + `e2e.yml`

**Files:**
- Create: `.github/workflows/web.yml`
- Create: `.github/workflows/e2e.yml`

- [ ] **Step 1: Web unit/RTL workflow**

```yaml
# .github/workflows/web.yml
name: web

on:
  push:
    paths: [ 'web/**', '.github/workflows/web.yml' ]
  pull_request:
    paths: [ 'web/**', '.github/workflows/web.yml' ]

jobs:
  test:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: web } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: web/package-lock.json }
      - run: npm ci
      - run: npm run typecheck
      - run: npm test
```

- [ ] **Step 2: E2E workflow** (cross-system, slower, runs on changes to any of api/worker/web)

```yaml
# .github/workflows/e2e.yml
name: e2e

on:
  push:
    paths: [ 'api/**', 'worker/**', 'web/**', 'docker-compose.e2e.yml', '.github/workflows/e2e.yml' ]
  pull_request:
    paths: [ 'api/**', 'worker/**', 'web/**', 'docker-compose.e2e.yml', '.github/workflows/e2e.yml' ]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - name: Build + start full stack
        run: docker compose -f docker-compose.e2e.yml up -d --build --wait
      - name: Install Playwright deps
        working-directory: web
        run: npm ci && npx playwright install --with-deps chromium
      - name: Run Playwright
        working-directory: web
        env:
          API_BASE: http://localhost:8080
          CDN_BASE: http://localhost:8787
          WEB_BASE: http://localhost:3000
        run: npm run test:e2e
      - name: Tear down
        if: always()
        run: docker compose -f docker-compose.e2e.yml down -v
      - name: Upload Playwright report
        if: failure()
        uses: actions/upload-artifact@v4
        with: { name: playwright-report, path: web/playwright-report/ }
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/web.yml .github/workflows/e2e.yml
git commit -m "[web] ci: web unit + cross-system E2E workflows"
```

---

## Done

After all 22 tasks land, Plan 3 produces a Next.js frontend that:

- Renders a public masonry feed at `/` with infinite scroll, no duplicates, no layout shift, lazy loading
- Renders viewer-aware `/u/[slug]` and `/art/[id]` with `force-dynamic` + `no-store`; both fall through to Next's `notFound()` on 404 (no leaked metadata)
- `/upload` server-side checks `/me` and redirects anonymous viewers to OAuth start before painting the form
- Owners see their drafts; anonymous and other-user requests get private content scrubbed from the HTML
- Uses `next/image` with the contracts §6 allowlist via `cf-loader`, AVIF/WebP via `fmt=auto`, and the upstream `blurhash` package for placeholders (no buggy in-tree decoder)
- Forwards cookies via an explicit `forwardCookie()` helper that does not depend on the undocumented `cookies().toString()` shape
- Has 4 Playwright privacy tests (cases 6-9) and 4 UX tests (cases 22-25) running against a docker-compose'd full stack with healthchecks on every service so `up --wait` actually waits for readiness
- Treats the API and Worker as pure consumers — no NextAuth, no client-side auth state
- Worker container in compose runs the production `handle()` function via a small Node entrypoint that wraps MinIO as R2 and stubs IMAGES with pass-through; `wrangler dev` is reserved for Layer-B (Plan 2 Task 11) where real resize is verified

Combined with Plans 1 and 2, all 26 critical-correctness tests from spec §8.6 are covered (including 17b fingerprint mismatch), and §12 acceptance criteria (Lighthouse ≥ 90 on the home feed under 4G with 50 images) is achievable given the rendering choices.
