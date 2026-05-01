# Plan 2 — Cloudflare Worker Image Gateway

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Cloudflare Worker at `cdn.example.com/img/...` that authenticates private requests via HMAC, fetches source bytes from R2 via binding, transforms them via the `env.IMAGES` binding, and emits cache-correct responses. The Worker is the *single chokepoint* for image bytes — no other path in the system can read R2.

**Architecture:** A single Worker module in `worker/src/index.ts` (~120 lines) backed by three small helpers — `sign.ts` (HMAC verify), `path.ts` (canonical path validation), `allowlist.ts` (constants). Tests run in two layers: an in-process layer using `@cloudflare/vitest-pool-workers` (CI-friendly, no Cloudflare account needed; fakes the `IMAGES` binding) and an end-to-end layer against `wrangler dev` (verifies actual resize via real `env.IMAGES`). The Worker is **stateless** — no KV, no D1, no Durable Objects.

**Tech Stack:** TypeScript, Cloudflare Workers, `@cloudflare/vitest-pool-workers`, `wrangler` (deploy + dev), `image-size` (pure-JS dimension probe in Layer-B tests).

**Subtree owned by this plan:** `worker/` (read [contracts §1](./2026-04-30-art-gallery-contracts.md#1-monorepo-layout)).

**Read first:**
1. [Design spec](../specs/2026-04-30-art-gallery-design.md) — sections 8.1, 8.2, 8.3, 8.6.1 (rows 10–14), 8.6.2
2. [Contracts doc](./2026-04-30-art-gallery-contracts.md) — sections 3, 4, 5, 6, 10, 12

**Prerequisite tools:** Node 20+, `npm` 10+, optionally `wrangler` 3.x (only for Layer-B integration tests). No Cloudflare account needed for Layer A.

---

## Task overview

| # | Task | Tests change |
|---|---|---|
| 1 | Bootstrap `worker/` package | smoke build |
| 2 | Allowlist constants module | unit |
| 3 | Canonical path validator | unit |
| 4 | HMAC verifier (WebCrypto) | unit |
| 5 | Worker fetch handler — full handler with public path + IMAGES test fake + JPEG fixture | unit |
| 6 | Worker fetch handler — private path with HMAC verify | unit |
| 7 | Additional transform-call assertions (fmt negotiation, q default) | unit |
| 8 | Privacy matrix tests (cases 10–14) | unit |
| 9 | Cache-key safety test (case 16) | unit |
| 10 | Test signer (TS port of Plan 1's signer) | unit |
| 11 | Layer-B round-trip integration test (case 15) | integration |
| 12 | E2E entrypoint (`scripts/e2e-server.ts`) + `Dockerfile.e2e` | smoke |
| 13 | `.github/workflows/worker.yml` | CI |

---

## Task 1: Bootstrap `worker/` package

**Files:**
- Create: `worker/package.json`
- Create: `worker/tsconfig.json`
- Create: `worker/wrangler.toml`
- Create: `worker/vitest.config.ts`
- Create: `worker/src/index.ts` (placeholder)
- Create: `worker/.gitignore`

- [ ] **Step 1: package.json**

```json
{
  "name": "art-web-worker",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "engines": {
    "node": ">=20"
  },
  "scripts": {
    "dev": "wrangler dev",
    "test": "vitest run",
    "test:integration": "vitest run -c vitest.integration.config.ts",
    "deploy": "wrangler deploy",
    "typecheck": "tsc --noEmit"
  },
  "devDependencies": {
    "@cloudflare/vitest-pool-workers": "^0.5.0",
    "@cloudflare/workers-types": "^4.20240117.0",
    "image-size": "^1.1.1",
    "typescript": "^5.4.0",
    "vitest": "^1.6.0",
    "wrangler": "^3.50.0"
  }
}
```

- [ ] **Step 2: tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "es2022",
    "lib": ["es2022"],
    "module": "es2022",
    "moduleResolution": "bundler",
    "types": ["@cloudflare/workers-types/experimental"],
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*", "test/**/*"]
}
```

- [ ] **Step 3: wrangler.toml**

```toml
name = "art-web-worker"
main = "src/index.ts"
compatibility_date = "2026-04-01"
compatibility_flags = ["nodejs_compat"]

[[r2_buckets]]
binding = "R2"
bucket_name = "art-prod"
preview_bucket_name = "art-dev"

[images]
binding = "IMAGES"

# WORKER_SIGNING_KEY is a secret, set via:
#   wrangler secret put WORKER_SIGNING_KEY
# It is the hex-encoded byte-identical key the Go API also holds (contracts §11).

[dev]
ip = "127.0.0.1"
port = 8787
```

- [ ] **Step 4: vitest.config.ts (Layer A — in-process tests, minimal)**

This file is rewritten in Task 6 once `WORKER_SIGNING_KEY` is needed. Tasks 2–5 don't read `env.WORKER_SIGNING_KEY`, so the bare config below is enough to compile and run early-task tests.

```ts
// worker/vitest.config.ts
import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    poolOptions: {
      workers: {
        wrangler: { configPath: "./wrangler.toml" },
        miniflare: {
          r2Buckets: ["R2"],
          // IMAGES is intentionally not bound — Layer-A tests inject a
          // fake via `mkTestEnv()` (Task 5). Layer-B (Task 11) hits the
          // real binding under `wrangler dev`.
        },
      },
    },
  },
});
```

- [ ] **Step 5: Placeholder `src/index.ts`**

```ts
// worker/src/index.ts
export default {
  async fetch(_req: Request, _env: unknown): Promise<Response> {
    return new Response("not yet implemented", { status: 501 });
  },
};
```

- [ ] **Step 6: .gitignore**

```gitignore
node_modules/
.wrangler/
.dev.vars
*.log
dist/
```

- [ ] **Step 7: Install + verify**

```bash
cd worker && npm install && npm run typecheck
```

- [ ] **Step 8: Commit**

```bash
git add worker/
git commit -m "[worker] chore: bootstrap Worker package with vitest-pool-workers"
```

---

## Task 2: Allowlist constants module

**Files:**
- Create: `worker/src/allowlist.ts`
- Create: `worker/test/allowlist.spec.ts`

- [ ] **Step 1: Failing test**

```ts
// worker/test/allowlist.spec.ts
import { describe, it, expect } from "vitest";
import { ALLOWED_WIDTHS, ALLOWED_FORMATS, ALLOWED_QUALITIES, isAllowedWidth } from "../src/allowlist";

describe("allowlist", () => {
  it("matches contracts §6 widths exactly", () => {
    expect([...ALLOWED_WIDTHS].sort((a, b) => a - b))
      .toEqual([240, 480, 800, 1024, 1600, 2400]);
  });
  it("matches contracts §6 formats exactly", () => {
    expect([...ALLOWED_FORMATS].sort()).toEqual(["auto", "avif", "jpeg", "webp"]);
  });
  it("matches contracts §6 qualities exactly", () => {
    expect([...ALLOWED_QUALITIES].sort((a, b) => a - b)).toEqual([60, 75, 85, 90]);
  });
  it("isAllowedWidth rejects out-of-set", () => {
    expect(isAllowedWidth(800)).toBe(true);
    expect(isAllowedWidth(801)).toBe(false);
    expect(isAllowedWidth(99999)).toBe(false);
  });
});
```

- [ ] **Step 2: Implementation**

```ts
// worker/src/allowlist.ts
// Mirrors contracts §6 byte-for-byte. Any change here must also land
// in web/lib/cf-loader.ts AND web/next.config.js deviceSizes.
export const ALLOWED_WIDTHS    = new Set<number>([240, 480, 800, 1024, 1600, 2400]);
export const ALLOWED_FORMATS   = new Set<string>(["auto", "avif", "webp", "jpeg"]);
export const ALLOWED_QUALITIES = new Set<number>([60, 75, 85, 90]);

export function isAllowedWidth(w: number): boolean {
  return Number.isFinite(w) && ALLOWED_WIDTHS.has(w);
}
```

- [ ] **Step 3: Run + commit**

```bash
cd worker && npm test -- allowlist
git add worker/src/allowlist.ts worker/test/allowlist.spec.ts
git commit -m "[worker] feat: allowlist constants matching contracts §6"
```

---

## Task 3: Canonical path validator

**Files:**
- Create: `worker/src/path.ts`
- Create: `worker/test/path.spec.ts`

This is the byte-for-byte mirror of `api/internal/auth/sign.go`'s `ValidateCanonicalPath`. The Go test fixed-vector (Plan 1 Task 8) and the TypeScript fixture here MUST agree on what's valid.

- [ ] **Step 1: Failing test**

```ts
// worker/test/path.spec.ts
import { describe, it, expect } from "vitest";
import { validateCanonicalPath } from "../src/path";

describe("validateCanonicalPath", () => {
  const good = [
    "/private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0a1b2c3d-4e5f-6789-abcd-ef0123456789.jpg",
    "/public/aaa/bbb.png",
  ];
  for (const p of good) {
    it(`accepts ${p}`, () => expect(validateCanonicalPath(p)).toBeNull());
  }

  const bad = [
    ["", "empty"],
    ["private/x.jpg", "no leading slash"],
    ["/private/x.jpg/", "trailing slash"],
    ["/private//x.jpg", "double slash"],
    ["/private/../etc.jpg", "dot-dot"],
    ["/private/./x.jpg", "dot"],
    ["/private/abc def.jpg", "space"],
    ["/private/abc%20.jpg", "percent"],
    ["/private/abc?q=1.jpg", "querychar"],
  ];
  for (const [p, why] of bad) {
    it(`rejects ${p} (${why})`, () => expect(validateCanonicalPath(p)).not.toBeNull());
  }
});
```

- [ ] **Step 2: Implementation**

```ts
// worker/src/path.ts
const ALLOWED_RE = /^[A-Za-z0-9/._-]+$/;

export function validateCanonicalPath(p: string): string | null {
  if (!p || p[0] !== "/") return "must start with /";
  if (p.endsWith("/")) return "must not end with /";
  if (p.includes("//") || p.includes("/../") || p.includes("/./"))
    return "contains forbidden segment";
  if (!ALLOWED_RE.test(p)) return "char not allowed";
  return null;
}
```

- [ ] **Step 3: Run + commit**

```bash
cd worker && npm test -- path
git add worker/src/path.ts worker/test/path.spec.ts
git commit -m "[worker] feat: canonical path validator matching contracts §3"
```

---

## Task 4: HMAC verifier (WebCrypto)

**Files:**
- Create: `worker/src/sign.ts`
- Create: `worker/test/sign.spec.ts`

This is the verifier counterpart to Plan 1 Task 8's signer. The string-to-sign format is contracts §3: `v1|<canonicalPath>|<exp>`.

- [ ] **Step 1: Failing tests including the same fixed vector locked in Plan 1 Task 8**

```ts
// worker/test/sign.spec.ts
import { describe, it, expect } from "vitest";
import { verifySignature } from "../src/sign";

// The Go signer test in Plan 1 uses the literal ASCII string
// "0123456789abcdef0123456789abcdef" as the key (32 bytes). We use the
// same bytes here so the locked HMAC vector matches.
const TEST_KEY_BYTES = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

describe("verifySignature", () => {
  it("accepts the locked vector from Plan 1 Task 8", async () => {
    // Paste the hex produced by the Go test; this is the contract anchor.
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000000, expected);
    expect(ok).toBe(true);
  });

  it("rejects a tampered signature", async () => {
    // Flip one nibble of a known-good signature; expect rejection.
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const tampered = expected.slice(0, -1) + (expected.slice(-1) === "0" ? "1" : "0");
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000000, tampered);
    expect(ok).toBe(false);
  });

  it("rejects a different canonical path", async () => {
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/ccc.jpg", 1700000000, expected);
    expect(ok).toBe(false);
  });

  it("rejects a different exp", async () => {
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000001, expected);
    expect(ok).toBe(false);
  });
});
```

The hex constant above (`c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654`) matches the locked Go vector in `api/internal/auth/sign_test.go::TestSign_FixedVector` (Plan 1 Task 8). This is the cross-language anchor — if either side recomputes a different value, the test fails on first run with no manual fill-in step.

- [ ] **Step 2: Implementation using WebCrypto**

```ts
// worker/src/sign.ts
const enc = new TextEncoder();

async function importKey(rawBytes: Uint8Array): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    "raw",
    rawBytes,
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign", "verify"],
  );
}

function bytesToHex(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let out = "";
  for (let i = 0; i < bytes.length; i++) {
    out += bytes[i].toString(16).padStart(2, "0");
  }
  return out;
}

export async function computeSignature(
  keyBytes: Uint8Array,
  canonicalPath: string,
  exp: number,
): Promise<string> {
  const stringToSign = `v1|${canonicalPath}|${exp}`;
  const key = await importKey(keyBytes);
  const sig = await crypto.subtle.sign("HMAC", key, enc.encode(stringToSign));
  return bytesToHex(sig);
}

export async function verifySignature(
  keyBytes: Uint8Array,
  canonicalPath: string,
  exp: number,
  presented: string,
): Promise<boolean> {
  const expected = await computeSignature(keyBytes, canonicalPath, exp);
  return constantTimeEqualHex(expected, presented);
}

// Constant-time hex comparison. WebCrypto has no built-in for hex strings,
// so we walk the bytes ourselves. Both inputs MUST be the same length;
// length mismatch returns false without short-circuiting timing.
export function constantTimeEqualHex(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) {
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return diff === 0;
}
```

- [ ] **Step 3: Run + commit**

```bash
cd worker && npm test -- sign
git add worker/src/sign.ts worker/test/sign.spec.ts
git commit -m "[worker] feat: WebCrypto HMAC verifier with constant-time hex compare"
```

---

## Task 5: Worker fetch handler — full handler with public path + IMAGES test fake

**Files:**
- Modify: `worker/src/index.ts`
- Create: `worker/test/_imagesFake.ts`
- Create: `worker/test/_env.ts`
- Create: `worker/test/fixtures/tiny.jpg` (binary, ~1 KB)
- Create: `worker/test/handler_public.spec.ts`

**Why this task pulls in the IMAGES fake:** the `env.IMAGES` binding is not natively faked by `vitest-pool-workers` — there's no offline implementation in miniflare. We could split the public-path handler from the IMAGES fake into separate tasks, but then Task 5's success-path test ("200 + Cache-Control") cannot be authored or verified. Pulling the fake in here keeps every task green when it lands and avoids a previous version of this plan that had unreachable Cache-Control logic and a `globalThis` side-channel that doesn't cross the workerd isolate boundary.

**The injection pattern:** `index.ts` exports both a `handle(req, env)` function and the conventional `default { fetch: handle }`. Tests import `handle` directly and pass an `env` object built from `cloudflare:test`'s real `env` (for R2 + WORKER_SIGNING_KEY) spread with a fake `IMAGES` binding. No globals, no module-level mutable state.

- [ ] **Step 1: IMAGES test fake**

```ts
// worker/test/_imagesFake.ts
// Records each transform pipeline call so tests can assert width/fmt/q
// shape. The output is a pass-through of the source stream — Layer-A tests
// don't verify pixel-level resize (that's Layer-B, Task 11).
export type ImagesCall = { width?: number; format: string; quality: number };

export class ImagesFake {
  public calls: ImagesCall[] = [];

  input(stream: ReadableStream): ImagesPipeline {
    return new ImagesPipeline(this, stream);
  }
}

class ImagesPipeline {
  private widthHint?: number;
  constructor(private fake: ImagesFake, private stream: ReadableStream) {}
  transform(opts: { width?: number }): ImagesPipeline {
    this.widthHint = opts.width;
    return this;
  }
  async output(opts: { format: string; quality: number }): Promise<{ response(): Response }> {
    this.fake.calls.push({ width: this.widthHint, format: opts.format, quality: opts.quality });
    return {
      response: () => new Response(this.stream, {
        status: 200,
        headers: { "Content-Type": opts.format },
      }),
    };
  }
}
```

- [ ] **Step 2: Test env helper**

```ts
// worker/test/_env.ts
// `wpwEnv` is the worker's bindings as configured by `vitest.config.ts` —
// real workerd-bound R2 and WORKER_SIGNING_KEY. We spread it so tests get
// the real R2 (so .put / .get round-trip) and override IMAGES with a fake.
import { env as wpwEnv } from "cloudflare:test";
import { ImagesFake } from "./_imagesFake";
import type { Env } from "../src/index";

export type TestEnv = Env & { _images: ImagesFake };

export function mkTestEnv(): TestEnv {
  const fake = new ImagesFake();
  return {
    ...(wpwEnv as unknown as Env),
    IMAGES: fake as unknown as Env["IMAGES"],
    _images: fake,
  };
}
```

- [ ] **Step 3: JPEG fixture**

The tests write source bytes into R2 so the handler can read them and feed them to `IMAGES.input(stream)`. The fake passes the bytes through unchanged, so technically any non-empty buffer works for Layer-A — but a real ~1 KB JPEG keeps the fixture honest in case a future test decodes the response (e.g. with `image-size`) before Layer-B runs.

Generate once with ImageMagick and commit:

```bash
cd worker && mkdir -p test/fixtures
convert -size 16x16 xc:steelblue test/fixtures/tiny.jpg
```

(The same `tiny.jpg` is reused by Tasks 6, 8, and 9; Task 11 ships a separate `2400px.jpg` because the round-trip test verifies pixel width.)

- [ ] **Step 4: Failing tests**

```ts
// worker/test/handler_public.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { handle } from "../src/index";
import { mkTestEnv, type TestEnv } from "./_env";

const TINY_JPEG = await readFile(fileURLToPath(new URL("./fixtures/tiny.jpg", import.meta.url)));

let env: TestEnv;
beforeEach(async () => {
  env = mkTestEnv();
  await env.R2.put("public/abc/img1.jpg", TINY_JPEG, {
    httpMetadata: { contentType: "image/jpeg" },
  });
});

describe("public path", () => {
  it("returns 200 + immutable Cache-Control on success", async () => {
    const r = await handle(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=800"), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("public, max-age=31536000, immutable");
    expect(env._images.calls).toEqual([{ width: 800, format: "image/jpeg", quality: 85 }]);
  });

  it("returns 404 when R2 object is missing", async () => {
    const r = await handle(
      new Request("https://cdn.example.com/img/public/abc/missing.jpg"), env);
    expect(r.status).toBe(404);
  });

  it("returns 404 for path outside /img/", async () => {
    const r = await handle(new Request("https://cdn.example.com/somewhere"), env);
    expect(r.status).toBe(404);
  });

  it("returns 400 for canonicalization-ambiguous path", async () => {
    const r = await handle(
      new Request("https://cdn.example.com/img/public/../foo.jpg"), env);
    expect(r.status).toBe(400);
  });

  it("rejects non-exact numeric transform params", async () => {
    for (const qs of ["w=800junk", "q=85x"]) {
      const r = await handle(new Request(`https://cdn.example.com/img/public/abc/img1.jpg?${qs}`), env);
      expect(r.status).toBe(400);
    }
  });
});
```

- [ ] **Step 5: Implement handler**

```ts
// worker/src/index.ts
import { ALLOWED_FORMATS, ALLOWED_QUALITIES, isAllowedWidth } from "./allowlist";
import { validateCanonicalPath } from "./path";
import { verifySignature } from "./sign";

const IMG_PREFIX = "/img/";

export interface Env {
  R2: R2Bucket;
  IMAGES: ImagesBinding;
  WORKER_SIGNING_KEY: string; // hex-encoded
}

export interface ImagesBinding {
  input(stream: ReadableStream): ImagesPipeline;
}
export interface ImagesPipeline {
  transform(opts: { width?: number }): ImagesPipeline;
  output(opts: { format: string; quality?: number }): Promise<ImagesOutput>;
}
export interface ImagesOutput {
  response(): Response;
}

// Single-pass handler. Each branch terminates with its own Response so a
// later branch never overwrites a 4xx body with a 200's Cache-Control —
// the bug class an earlier draft of this plan introduced via a shared
// `readAndTransform` helper.
export async function handle(request: Request, env: Env): Promise<Response> {
  const url = new URL(request.url);
  if (!url.pathname.startsWith(IMG_PREFIX)) {
    return new Response("Not found", { status: 404 });
  }
  const canonicalPath = "/" + url.pathname.slice(IMG_PREFIX.length);
  if (validateCanonicalPath(canonicalPath)) {
    return new Response("Bad request", { status: 400 });
  }

  let isPrivate: boolean;
  if (canonicalPath.startsWith("/private/")) {
    // Private branch — Task 6 replaces this stub with HMAC verification.
    return new Response("Unauthorized", { status: 401 });
  } else if (canonicalPath.startsWith("/public/")) {
    isPrivate = false;
  } else {
    return new Response("Not found", { status: 404 });
  }

  // Validate transform params before any R2 read so a malformed request
  // never racks up R2 GET cost.
  const wRaw = url.searchParams.get("w");
  let w: number | undefined;
  if (wRaw !== null) {
    const parsed = parseExactDecimal(wRaw);
    if (parsed === null) return new Response("Bad request: w must be a decimal integer", { status: 400 });
    if (!isAllowedWidth(parsed)) return new Response("Bad request: w not in allowlist", { status: 400 });
    w = parsed;
  }
  const fmtParam = url.searchParams.get("fmt") ?? "auto";
  if (!ALLOWED_FORMATS.has(fmtParam)) return new Response("Bad request: fmt not in allowlist", { status: 400 });
  const qRaw = url.searchParams.get("q");
  const q = qRaw === null ? 85 : parseExactDecimal(qRaw);
  if (q === null) return new Response("Bad request: q must be a decimal integer", { status: 400 });
  if (!ALLOWED_QUALITIES.has(q)) return new Response("Bad request: q not in allowlist", { status: 400 });

  const r2Key = canonicalPath.slice(1);
  const obj = await env.R2.get(r2Key);
  if (!obj) return new Response("Not found", { status: 404 });

  const fmt = fmtParam === "auto" ? negotiateFormat(request) : `image/${fmtParam}`;
  let pipeline = env.IMAGES.input(obj.body);
  if (w !== undefined) pipeline = pipeline.transform({ width: w });
  const transformed = (await pipeline.output({ format: fmt, quality: q })).response();

  const headers = new Headers(transformed.headers);
  headers.set(
    "Cache-Control",
    isPrivate ? "private, no-store" : "public, max-age=31536000, immutable",
  );
  return new Response(transformed.body, { status: transformed.status, headers });
}

export default {
  fetch: handle,
};

function parseExactDecimal(raw: string): number | null {
  if (!/^(0|[1-9][0-9]*)$/.test(raw)) return null;
  const n = Number(raw);
  return Number.isSafeInteger(n) ? n : null;
}

function negotiateFormat(request: Request): string {
  const accept = request.headers.get("Accept") || "";
  if (accept.includes("image/avif")) return "image/avif";
  if (accept.includes("image/webp")) return "image/webp";
  return "image/jpeg";
}

export function hexToBytes(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}
```

- [ ] **Step 6: Run + commit**

```bash
cd worker && npm test -- handler_public
git add worker/src/index.ts worker/test/_imagesFake.ts worker/test/_env.ts worker/test/fixtures/tiny.jpg worker/test/handler_public.spec.ts
git commit -m "[worker] feat: full fetch handler with public path + IMAGES test fake"
```

---

## Task 6: Worker fetch handler — private path

**Files:**
- Modify: `worker/src/index.ts`
- Modify: `worker/vitest.config.ts` (bind `WORKER_SIGNING_KEY`)
- Create: `worker/test/_signer.ts` (test-only signer helper)
- Create: `worker/test/handler_private.spec.ts`

- [ ] **Step 1: Test-only signer — same algorithm as Plan 1's signer**

```ts
// worker/test/_signer.ts
// In-test ONLY. Never imported from src/. The Worker holds verification
// logic only — never signing — so an attacker who breaches the Worker
// cannot mint new signed URLs.
import { computeSignature } from "../src/sign";

export async function makeSignedURL(
  base: string, // e.g. "https://cdn.example.com"
  keyBytes: Uint8Array,
  storageKey: string, // e.g. "private/abc/img1.jpg"
  expSeconds: number,
  query: Record<string, string> = {},
): Promise<string> {
  const canonical = "/" + storageKey;
  const sig = await computeSignature(keyBytes, canonical, expSeconds);
  const u = new URL(`${base}/img${canonical}`);
  u.searchParams.set("sig", sig);
  u.searchParams.set("exp", String(expSeconds));
  for (const [k, v] of Object.entries(query)) u.searchParams.set(k, v);
  return u.toString();
}
```

- [ ] **Step 2: Bind `WORKER_SIGNING_KEY` in `vitest.config.ts`**

The Go signer test in Plan 1 uses the literal ASCII string `"0123456789abcdef0123456789abcdef"` as the key (32 ASCII bytes). The Worker reads `env.WORKER_SIGNING_KEY` as a hex-encoded string and decodes it via `hexToBytes`. Each ASCII char of the source key → 2 hex chars, so the env value is exactly 64 hex chars: 32 chars from `"0123456789abcdef"` repeated twice.

```ts
// worker/vitest.config.ts (replace contents)
import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    poolOptions: {
      workers: {
        wrangler: { configPath: "./wrangler.toml" },
        miniflare: {
          bindings: {
            WORKER_SIGNING_KEY:
              "30313233343536373839616263646566" +
              "30313233343536373839616263646566", // 64 hex chars = ASCII "0123456789abcdef" twice
          },
          r2Buckets: ["R2"],
          // IMAGES is not bound by miniflare — Layer-A tests inject the
          // ImagesFake via `mkTestEnv()` (Task 5 step 2). Layer-B tests
          // (Task 11) hit the real binding under `wrangler dev`.
        },
      },
    },
  },
});
```

- [ ] **Step 3: Failing tests — covers cases 11, 12, 13, 14 from §8.6.1**

```ts
// worker/test/handler_private.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { handle } from "../src/index";
import { mkTestEnv, type TestEnv } from "./_env";
import { makeSignedURL } from "./_signer";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");
const TINY_JPEG = await readFile(fileURLToPath(new URL("./fixtures/tiny.jpg", import.meta.url)));

let env: TestEnv;
beforeEach(async () => {
  env = mkTestEnv();
  await env.R2.put("private/abc/img1.jpg", TINY_JPEG, {
    httpMetadata: { contentType: "image/jpeg" },
  });
});

describe("private path", () => {
  it("case 11 — no sig returns 401", async () => {
    const r = await handle(
      new Request("https://cdn.example.com/img/private/abc/img1.jpg?w=800"), env);
    expect(r.status).toBe(401);
  });

  it("case 12 — valid sig + future exp returns 200 with private/no-store", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, { w: "800" });
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("private, no-store");
  });

  it("rejects expired exp with 401", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) - 60, {});
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(401);
  });

  it("rejects tampered sig with 401", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, {});
    const tampered = url.replace(/sig=([0-9a-f])/, (_, c) => `sig=${c === "0" ? "1" : "0"}`);
    const r = await handle(new Request(tampered), env);
    expect(r.status).toBe(401);
  });

  it("case 13 — out-of-allowlist width returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, { w: "99999" });
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(400);
  });

  it("rejects non-exact exp values before signature verification", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, {});
    const u = new URL(url);
    u.searchParams.set("exp", `${u.searchParams.get("exp")}x`);
    const r = await handle(new Request(u.toString()), env);
    expect(r.status).toBe(401);
  });

  it("case 14 — out-of-allowlist fmt returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, { fmt: "svg" });
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(400);
  });
});
```

- [ ] **Step 4: Replace the placeholder private branch in `src/index.ts`**

In `handle()`, replace:

```ts
  if (canonicalPath.startsWith("/private/")) {
    // Private branch — Task 6 replaces this stub with HMAC verification.
    return new Response("Unauthorized", { status: 401 });
  } else if (canonicalPath.startsWith("/public/")) {
```

with the real verification:

```ts
  let isPrivate: boolean;
  if (canonicalPath.startsWith("/private/")) {
    isPrivate = true;
    const sig = url.searchParams.get("sig");
    const expStr = url.searchParams.get("exp");
    if (!sig || !expStr) return new Response("Unauthorized", { status: 401 });
    const exp = parseExactDecimal(expStr);
    if (exp === null || Date.now() / 1000 > exp) {
      return new Response("Unauthorized", { status: 401 });
    }
    const keyBytes = hexToBytes(env.WORKER_SIGNING_KEY);
    const ok = await verifySignature(keyBytes, canonicalPath, exp, sig);
    if (!ok) return new Response("Unauthorized", { status: 401 });
  } else if (canonicalPath.startsWith("/public/")) {
```

(The `let isPrivate: boolean;` declaration moves up next to the branch that needs to differentiate, since after Task 6 the private branch flows through to the same R2 + transform path the public branch uses. Remove the now-orphaned `let isPrivate: boolean;` declaration that previously sat below the if/else.)

- [ ] **Step 5: Run + commit**

```bash
cd worker && npm test -- handler_private
git add worker/src/index.ts worker/vitest.config.ts worker/test/_signer.ts worker/test/handler_private.spec.ts
git commit -m "[worker] feat: private path with HMAC verify (cases 11-14)"
```

---

## Task 7: Additional transform-call assertions

**Files:**
- Create: `worker/test/handler_transform.spec.ts`

The `ImagesFake` and `mkTestEnv()` helper landed in Task 5 (where they were needed for the public-path success test). This task adds the assertions that pin format negotiation and quality forwarding to the contracts §6 allowlist — separate from Task 5 because they're orthogonal to the success path and to keep each spec file focused.

- [ ] **Step 1: Failing tests**

```ts
// worker/test/handler_transform.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { handle } from "../src/index";
import { mkTestEnv, type TestEnv } from "./_env";

const TINY_JPEG = await readFile(fileURLToPath(new URL("./fixtures/tiny.jpg", import.meta.url)));

let env: TestEnv;
beforeEach(async () => {
  env = mkTestEnv();
  await env.R2.put("public/abc/img1.jpg", TINY_JPEG, {
    httpMetadata: { contentType: "image/jpeg" },
  });
});

describe("transform call shape", () => {
  it("forwards explicit fmt=webp + q=90 to the binding", async () => {
    await handle(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=800&fmt=webp&q=90"), env);
    expect(env._images.calls).toEqual([{ width: 800, format: "image/webp", quality: 90 }]);
  });

  it("auto fmt + Accept: image/avif → image/avif passed through", async () => {
    await handle(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=240",
        { headers: { Accept: "image/avif,image/*" } }), env);
    expect(env._images.calls[0].format).toBe("image/avif");
  });

  it("auto fmt + Accept: image/webp → image/webp passed through", async () => {
    await handle(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=240",
        { headers: { Accept: "image/webp,*/*" } }), env);
    expect(env._images.calls[0].format).toBe("image/webp");
  });

  it("auto fmt with no Accept → image/jpeg fallback", async () => {
    await handle(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=240"), env);
    expect(env._images.calls[0].format).toBe("image/jpeg");
  });

  it("missing q defaults to 85", async () => {
    await handle(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=800&fmt=jpeg"), env);
    expect(env._images.calls[0].quality).toBe(85);
  });
});
```

- [ ] **Step 2: Run + commit**

```bash
cd worker && npm test -- handler_transform
git add worker/test/handler_transform.spec.ts
git commit -m "[worker] test: transform-call shape assertions (fmt negotiation, q default)"
```

---

## Task 8: Privacy matrix tests (cases 10, 11, 12, 13, 14)

**Files:**
- Create: `worker/test/privacy_matrix.spec.ts`

This consolidates the cases already partially covered in Tasks 5+6 into a single suite that mirrors the design spec's matrix.

- [ ] **Step 1: Test**

```ts
// worker/test/privacy_matrix.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { handle } from "../src/index";
import { mkTestEnv, type TestEnv } from "./_env";
import { makeSignedURL } from "./_signer";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");
const TINY_JPEG = await readFile(fileURLToPath(new URL("./fixtures/tiny.jpg", import.meta.url)));

let env: TestEnv;
beforeEach(async () => {
  env = mkTestEnv();
  await env.R2.put("public/A/p.jpg",  TINY_JPEG, { httpMetadata: { contentType: "image/jpeg" } });
  await env.R2.put("private/A/q.jpg", TINY_JPEG, { httpMetadata: { contentType: "image/jpeg" } });
});

describe("privacy matrix (Worker rows)", () => {
  it("row 10 — public ?w=800 returns 200, immutable cache", async () => {
    const r = await handle(
      new Request("https://cdn.example.com/img/public/A/p.jpg?w=800"), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("public, max-age=31536000, immutable");
    expect(env._images.calls[0].width).toBe(800);
  });

  it("row 11 — private without sig returns 401", async () => {
    const r = await handle(
      new Request("https://cdn.example.com/img/private/A/q.jpg?w=800"), env);
    expect(r.status).toBe(401);
  });

  it("row 12 — private with valid sig returns 200, no-store", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/A/q.jpg", Math.floor(Date.now()/1000) + 300, { w: "800" });
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("private, no-store");
    expect(env._images.calls[0].width).toBe(800);
  });

  it("row 13 — w=99999 returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/A/q.jpg", Math.floor(Date.now()/1000) + 300, { w: "99999" });
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(400);
  });

  it("row 14 — fmt=svg returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/A/q.jpg", Math.floor(Date.now()/1000) + 300, { fmt: "svg" });
    const r = await handle(new Request(url), env);
    expect(r.status).toBe(400);
  });
});
```

- [ ] **Step 2: Run + commit**

```bash
cd worker && npm test -- privacy_matrix
git add worker/test/privacy_matrix.spec.ts
git commit -m "[worker] test: privacy matrix rows 10-14 (spec §8.6.1)"
```

---

## Task 9: Cache-key safety test (case 16)

**Files:**
- Create: `worker/test/cache_safety.spec.ts`

The risk: a CDN/edge cache in front of the Worker could match owner-served bytes with a path-only key, then serve those bytes to an unsigned request. The Worker can't directly "test the CDN", but it can guarantee: (a) `Cache-Control: private, no-store` is set on every private response, (b) unsigned requests to private paths return 401 before any cache lookup the Worker would do.

This test asserts those two invariants hold even after a successful authenticated fetch to the same path.

- [ ] **Step 1: Test**

```ts
// worker/test/cache_safety.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { handle } from "../src/index";
import { mkTestEnv, type TestEnv } from "./_env";
import { makeSignedURL } from "./_signer";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");
const TINY_JPEG = await readFile(fileURLToPath(new URL("./fixtures/tiny.jpg", import.meta.url)));

let env: TestEnv;
beforeEach(async () => {
  env = mkTestEnv();
  await env.R2.put("private/X/img.jpg", TINY_JPEG, { httpMetadata: { contentType: "image/jpeg" } });
});

describe("case 16 — cache-key safety", () => {
  it("owner-then-anonymous: anon never receives owner bytes", async () => {
    // Owner request with valid sig.
    const ownerURL = await makeSignedURL("https://cdn.example.com", KEY,
      "private/X/img.jpg", Math.floor(Date.now()/1000) + 300, { w: "800" });
    const ownerResp = await handle(new Request(ownerURL), env);
    expect(ownerResp.status).toBe(200);
    expect(ownerResp.headers.get("Cache-Control")).toBe("private, no-store");

    // Anonymous request to the same path with no sig.
    const anonResp = await handle(
      new Request("https://cdn.example.com/img/private/X/img.jpg?w=800"), env);
    expect(anonResp.status).toBe(401);
  });

  it("owner-then-tampered: tampered sig never receives owner bytes", async () => {
    const ownerURL = await makeSignedURL("https://cdn.example.com", KEY,
      "private/X/img.jpg", Math.floor(Date.now()/1000) + 300, {});
    const ownerResp = await handle(new Request(ownerURL), env);
    expect(ownerResp.status).toBe(200);

    const tampered = ownerURL.replace(/sig=([0-9a-f])/, (_, c) =>
      "sig=" + (c === "0" ? "1" : "0"));
    const r = await handle(new Request(tampered), env);
    expect(r.status).toBe(401);
  });
});
```

- [ ] **Step 2: Run + commit**

```bash
cd worker && npm test -- cache_safety
git add worker/test/cache_safety.spec.ts
git commit -m "[worker] test: cache-key safety regression (case 16)"
```

---

## Task 10: Test signer correctness pin

**Files:**
- Modify: `worker/test/sign.spec.ts`
- Create: `worker/test/contract_pin.spec.ts`

The `_signer.ts` helper was added in Task 6. Now we **pin** the cross-language contract: a known input fed through the TS signer MUST produce the exact hex that the Go signer produced for the same input in Plan 1 Task 8.

- [ ] **Step 1: The pin test**

```ts
// worker/test/contract_pin.spec.ts
import { describe, it, expect } from "vitest";
import { computeSignature } from "../src/sign";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

describe("cross-language signature contract", () => {
  it("matches Plan 1 Task 8 fixed vector for /private/aaa/bbb.jpg @ 1700000000", async () => {
    const got = await computeSignature(KEY, "/private/aaa/bbb.jpg", 1700000000);
    expect(got).toBe("c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654");
  });
});
```

- [ ] **Step 2: Run** — passes on first run. The hex above is the locked vector from Plan 1 Task 8 (`c018a64c…3654`); first compile to first green, no manual fill-in step.

```bash
cd worker && npm test -- contract_pin
```

- [ ] **Step 3: Commit**

```bash
git add worker/test/contract_pin.spec.ts
git commit -m "[worker] test: pin cross-language signature contract to Plan 1 Task 8 vector"
```

---

## Task 11: Layer-B round-trip integration test (case 15)

**Files:**
- Create: `worker/vitest.integration.config.ts`
- Create: `worker/test_integration/round_trip.spec.ts`
- Create: `worker/test_integration/README.md`

This is the Layer-B test the spec describes: a real Worker (under `wrangler dev`) gets a signed URL from the **Go API** (Plan 1) and the test asserts (a) HTTP 200 and (b) decoded image width within 1px of the requested width. This requires both Plan 1 and Plan 2 to be running.

This test is **gated**: skipped automatically when the API URL or the Worker URL aren't reachable. CI runs it only when both are deployed.

- [ ] **Step 1: Integration vitest config (uses Node, not workerd)**

```ts
// worker/vitest.integration.config.ts
import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["test_integration/**/*.spec.ts"],
    environment: "node",
    testTimeout: 30000,
  },
});
```

- [ ] **Step 2: The round-trip test**

```ts
// worker/test_integration/round_trip.spec.ts
import { describe, it, expect } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import imageSize from "image-size";

const API   = process.env.ART_API_BASE   || "http://localhost:8080";
const CDN   = process.env.ART_CDN_BASE   || "http://localhost:8787";
const TOKEN = process.env.ART_OWNER_JWT  || "";

describe.skipIf(!TOKEN)("case 15 — signed-URL round-trip", () => {
  it("API-issued private URL renders 200 + correct width", async () => {
    // 1. Owner creates a private artwork and uploads one image.
    const created = await fetch(`${API}/artworks`, {
      method: "POST",
      headers: { Cookie: `auth=${TOKEN}`, "Content-Type": "application/json" },
      body: JSON.stringify({ title: "rt", visibility: "private" }),
    });
    const art = await created.json();

    // Read the 2400-px JPEG fixture (committed under test_integration/fixtures/).
    // `import.meta.url` is the standard ESM way to anchor relative paths;
    // `import.meta.dir` (Bun) and `fetch("file://...")` (browser) are not
    // available in Node, which is what `vitest.integration.config.ts` runs.
    const fixturePath = fileURLToPath(new URL("./fixtures/2400px.jpg", import.meta.url));
    const fixtureBytes = await readFile(fixturePath);

    const fd = new FormData();
    fd.set("manifest", JSON.stringify([
      { client_image_id: "K", position: 0, content_type: "image/jpeg" },
    ]));
    fd.set("files", new Blob([fixtureBytes], { type: "image/jpeg" }), "2400.jpg");
    await fetch(`${API}/artworks/${art.id}/images`, {
      method: "POST", headers: { Cookie: `auth=${TOKEN}` }, body: fd,
    });

    // 2. Fetch detail, extract the signed URL, swap CDN base if needed.
    const detail = await fetch(`${API}/artworks/${art.id}`, {
      headers: { Cookie: `auth=${TOKEN}` },
    }).then(r => r.json());
    const signed = detail.images[0].url
      .replace(/^https?:\/\/cdn\.example\.com/, CDN);

    // 3. GET signed?w=800 against the Worker; decode the bytes.
    const u = new URL(signed);
    u.searchParams.set("w", "800");
    u.searchParams.set("fmt", "jpeg");
    u.searchParams.set("q", "85");
    const resp = await fetch(u);
    expect(resp.status).toBe(200);
    const buf = Buffer.from(await resp.arrayBuffer());
    const dims = imageSize(buf);
    expect(dims.width).toBeGreaterThanOrEqual(799);
    expect(dims.width).toBeLessThanOrEqual(801);
  });
});
```

- [ ] **Step 3: Test fixtures**

Generate `test_integration/fixtures/2400px.jpg` with ImageMagick (a solid-color JPEG keeps the fixture small while preserving the 2400px width that drives the resize assertion):

```bash
cd worker && mkdir -p test_integration/fixtures
convert -size 2400x1600 xc:steelblue test_integration/fixtures/2400px.jpg
# Verify dimensions before committing (sanity check):
#   identify test_integration/fixtures/2400px.jpg
#   → test_integration/fixtures/2400px.jpg JPEG 2400x1600 ...
```

Commit the binary fixture (~30 KB at quality 85). The `convert` command is recorded in `test_integration/README.md` so anyone can regenerate it.

If you don't have ImageMagick, a Pillow one-liner works:

```bash
python3 -c "from PIL import Image; Image.new('RGB', (2400, 1600), 'steelblue').save('test_integration/fixtures/2400px.jpg', 'JPEG', quality=85)"
```

- [ ] **Step 4: README documenting how to run Layer B locally**

````markdown
# Layer-B integration tests

Runs the full round-trip described in spec §8.6.2 case 15.

**Prerequisites:**

1. Plan 1's API running locally with `APP_ENV=test`: `cd ../api && APP_ENV=test go run ./cmd/api` with `WORKER_SIGNING_KEY`, `JWT_SIGNING_KEY`, `DATABASE_URL`, `CDN_ORIGIN=http://localhost:8787`, and S3/R2 credentials exported. `APP_ENV=test` registers the `POST /dev/seed` endpoint, but **storage must still be shared with the Worker** for this Layer-B round-trip: set `S3_ENDPOINT` to the Cloudflare R2 S3 endpoint for the same dev bucket bound to wrangler, plus `R2_ACCESS_KEY_ID`, `R2_ACCESS_KEY_SECRET`, and `R2_BUCKET`. Do not leave `S3_ENDPOINT` unset here; that selects localfs for API-only tests and the Worker will 404 because it reads from R2.
2. This Worker running locally: `cd .. && npm run dev` (wrangler dev). Same `WORKER_SIGNING_KEY`. R2 bound to the **same dev bucket** the API writes via S3/R2 credentials; the `[images]` binding requires a Cloudflare account — wrangler 3.x supports an `experimental_remote = true` flag on the IMAGES binding (in `wrangler.toml` under `[dev]`) to proxy through real Cloudflare Images. Pin to `wrangler@^3.50.0` (per contracts §13.3) — earlier 3.x versions used a different flag name (`experimental_remote_bindings`) and the helper config we ship targets the newer name. If no Cloudflare R2 account is available, skip this Layer-B test locally and use Plan 3's MinIO-backed compose harness for storage-sharing coverage; compose does not prove real Cloudflare Images resizing.
3. Mint an owner JWT via the test-only seed endpoint (the manual "open Google in a browser" path is not deterministic and is reserved for end-user smoke testing):

```bash
SEED=$(curl -fsS -X POST http://localhost:8080/dev/seed)
ART_OWNER_JWT=$(echo "$SEED" | jq -r .aliceCookie | sed 's/^auth=//')
```

**Run:**

```bash
ART_API_BASE=http://localhost:8080 \
ART_CDN_BASE=http://localhost:8787 \
ART_OWNER_JWT=$ART_OWNER_JWT \
npm run test:integration
```
````

- [ ] **Step 5: Commit**

```bash
git add worker/vitest.integration.config.ts worker/test_integration/
git commit -m "[worker] test: layer-B round-trip with image-size width verification (case 15)"
```

---

## Task 12: E2E entrypoint + `Dockerfile.e2e` (consumed by `web/docker-compose.e2e.yml`)

**Files:**
- Create: `worker/scripts/e2e-server.ts`
- Create: `worker/Dockerfile.e2e`
- Modify: `worker/package.json` (add `@aws-sdk/client-s3` + `tsx` to devDependencies)
- Modify: `worker/package-lock.json` (generated by `npm install --save-dev`; required because Dockerfile uses `npm ci`)

**Why this lives in Plan 2:** the entrypoint imports the production `handle()` from `worker/src/index.ts` and shims a MinIO-backed S3 client into the R2 binding shape. Per contracts §1, files under `worker/` belong to Plan 2 — Plan 3 (which orchestrates the compose stack at `web/docker-compose.e2e.yml`) consumes this task's output via the `worker` service's `build:` directive.

**Why a Node entrypoint instead of `wrangler dev`:** `wrangler dev` requires `wrangler login` against Cloudflare which can't run unattended in CI. Miniflare standalone runs the Worker but cannot emulate the `env.IMAGES` binding offline. The clean compromise: a small Node entrypoint that reuses the production `handle()` so canonical-path + HMAC + Cache-Control logic still flows through real production code; only the actual pixel resize is mocked. Layer-B (Task 11) is what proves the resize works against real `env.IMAGES`.

- [ ] **Step 1: `scripts/e2e-server.ts`** — production `handle()` wrapper

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

// IMAGES stub — no real resize. Layer-B (Task 11) is what proves the real
// binding works; this stub just ensures privacy + Cache-Control logic flow
// through the production handler.
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

- [ ] **Step 2: `worker/Dockerfile.e2e`**

```dockerfile
# worker/Dockerfile.e2e
# E2E-only image. Production Worker deploys via `wrangler deploy` and
# never builds this image. Node version pinned per contracts §13.1.
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

- [ ] **Step 3: `package.json` devDependencies**

Run from `worker/` so `package.json` and `package-lock.json` stay in sync for the Dockerfile's `npm ci` step:

```bash
npm install --save-dev @aws-sdk/client-s3@^3.600.0 tsx@^4.7.0
```

These are devDependencies on purpose — production `wrangler deploy` must not bundle them. The deployed Worker uses `env.R2` (not the S3 client) and never executes `scripts/e2e-server.ts`.

- [ ] **Step 4: Smoke build**

```bash
cd worker && docker build -f Dockerfile.e2e -t art-web-worker-e2e:smoke .
```

- [ ] **Step 5: Commit**

```bash
git add worker/scripts/e2e-server.ts worker/Dockerfile.e2e worker/package.json worker/package-lock.json
git commit -m "[worker] feat: E2E entrypoint + Dockerfile.e2e (MinIO-backed R2 shim)"
```

---

## Task 13: `.github/workflows/worker.yml`

**Files:**
- Create: `.github/workflows/worker.yml`

- [ ] **Step 1: Workflow**

```yaml
# .github/workflows/worker.yml
name: worker

on:
  push:
    paths: [ 'worker/**', '.github/workflows/worker.yml' ]
  pull_request:
    paths: [ 'worker/**', '.github/workflows/worker.yml' ]

jobs:
  test:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: worker } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: worker/package-lock.json }
      - run: npm ci
      - run: npm run typecheck
      - run: npm test
```

Layer B is intentionally NOT part of `worker.yml` — it requires both Plan 1's API and Plan 2's Worker deployed. Layer B runs on `e2e.yml` (Plan 3 owns that file).

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/worker.yml
git commit -m "[worker] ci: GitHub Actions workflow scoped to worker/**"
```

---

## Done

After all 13 tasks land, Plan 2 produces a Cloudflare Worker that:

- Validates canonical paths (contracts §3) and rejects malformed input with 400
- Verifies HMAC signatures byte-perfect against Plan 1 (via the locked vector pin in Task 10)
- Reads R2 via binding, transforms via `env.IMAGES` with the allowlist (contracts §6), emits the right Cache-Control per visibility (set in the success path only — 4xx responses keep their own bodies/headers)
- Exposes a pure `handle(req, env)` function so tests inject a fake `IMAGES` via env-spread instead of relying on a `globalThis` side-channel that doesn't cross workerd's isolate boundary
- Passes privacy-matrix rows 10–14 (§8.6.1) and the cache-safety regression (§8.6.2 case 16) in CI without Cloudflare credentials
- Has a Layer-B round-trip test (case 15) that mints its owner cookie via Plan 1's `POST /dev/seed` (no manual Google OAuth step)

The Worker source is ~120 lines as the spec mandates. Anything that grows beyond that is a refactor signal.
