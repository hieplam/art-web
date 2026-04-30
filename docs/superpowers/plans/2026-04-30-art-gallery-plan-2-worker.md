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
| 5 | Worker fetch handler — public path | unit |
| 6 | Worker fetch handler — private path | unit |
| 7 | Worker fetch handler — cache headers | unit |
| 8 | Privacy matrix tests (cases 10–14) | unit |
| 9 | Cache-key safety test (case 16) | unit |
| 10 | Test signer (TS port of Plan 1's signer) | unit |
| 11 | Layer-B round-trip integration test (case 15) | integration |
| 12 | `.github/workflows/worker.yml` | CI |

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

- [ ] **Step 4: vitest.config.ts (Layer A — in-process tests)**

```ts
// worker/vitest.config.ts
import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    poolOptions: {
      workers: {
        wrangler: { configPath: "./wrangler.toml" },
        miniflare: {
          // Bind a fake IMAGES binding for tests that does pass-through.
          // Real resize is only exercised in Layer-B integration tests.
          serviceBindings: {},
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

const TEST_KEY_HEX = "30313233343536373839616263646566303132333435363738396162636465666"; // "0123456789abcdef" * 2 in hex
const TEST_KEY_BYTES = (() => {
  // The Go test uses the literal ASCII string "0123456789abcdef0123456789abcdef" as the key.
  // We replicate exactly: encode that ASCII as bytes.
  return new TextEncoder().encode("0123456789abcdef0123456789abcdef");
})();

describe("verifySignature", () => {
  it("accepts the locked vector from Plan 1 Task 8", async () => {
    // Paste the hex produced by the Go test; this is the contract anchor.
    const expected = "<PASTE-FROM-PLAN-1-TASK-8>";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000000, expected);
    expect(ok).toBe(true);
  });

  it("rejects a tampered signature", async () => {
    // Flip one nibble of a known-good signature; expect rejection.
    const expected = "<PASTE-FROM-PLAN-1-TASK-8>";
    const tampered = expected.slice(0, -1) + (expected.slice(-1) === "0" ? "1" : "0");
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000000, tampered);
    expect(ok).toBe(false);
  });

  it("rejects a different canonical path", async () => {
    const expected = "<PASTE-FROM-PLAN-1-TASK-8>";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/ccc.jpg", 1700000000, expected);
    expect(ok).toBe(false);
  });

  it("rejects a different exp", async () => {
    const expected = "<PASTE-FROM-PLAN-1-TASK-8>";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000001, expected);
    expect(ok).toBe(false);
  });
});
```

The placeholder `<PASTE-FROM-PLAN-1-TASK-8>` is replaced with the hex value the Go test produced. This is the cross-language anchor.

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

## Task 5: Worker fetch handler — public path

**Files:**
- Modify: `worker/src/index.ts`
- Create: `worker/test/handler_public.spec.ts`

- [ ] **Step 1: Failing test against an in-Worker R2 binding**

```ts
// worker/test/handler_public.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { env, SELF } from "cloudflare:test";
import worker from "../src/index";

beforeEach(async () => {
  // Seed R2 with a tiny JPEG.
  const jpeg = new Uint8Array([
    0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
    // ... minimal valid JPEG header; full bytes generated in Task 11 fixtures
  ]);
  await env.R2.put("public/abc/img1.jpg", jpeg, {
    httpMetadata: { contentType: "image/jpeg" },
  });
});

describe("public path", () => {
  it("returns 200 + correct Cache-Control for /img/public/...", async () => {
    const r = await worker.fetch(new Request("https://cdn.example.com/img/public/abc/img1.jpg"), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("public, max-age=31536000, immutable");
  });

  it("returns 404 when the R2 object is missing", async () => {
    const r = await worker.fetch(new Request("https://cdn.example.com/img/public/abc/missing.jpg"), env);
    expect(r.status).toBe(404);
  });

  it("returns 404 for path outside /img/", async () => {
    const r = await worker.fetch(new Request("https://cdn.example.com/somewhere"), env);
    expect(r.status).toBe(404);
  });
});
```

- [ ] **Step 2: Build the public branch of the handler**

```ts
// worker/src/index.ts
import { ALLOWED_WIDTHS, ALLOWED_FORMATS, ALLOWED_QUALITIES, isAllowedWidth } from "./allowlist";
import { validateCanonicalPath } from "./path";
import { verifySignature } from "./sign";

const IMG_PREFIX = "/img/";

interface Env {
  R2: R2Bucket;
  IMAGES: ImagesBinding;
  WORKER_SIGNING_KEY: string; // hex-encoded
}

interface ImagesBinding {
  input(stream: ReadableStream): ImagesPipeline;
}
interface ImagesPipeline {
  transform(opts: { width?: number }): ImagesPipeline;
  output(opts: { format: string; quality?: number }): Promise<ImagesOutput>;
}
interface ImagesOutput {
  response(): Response;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
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
      // Private branch — see Task 6.
      return new Response("Unauthorized", { status: 401 }); // placeholder
    } else if (canonicalPath.startsWith("/public/")) {
      isPrivate = false;
    } else {
      return new Response("Not found", { status: 404 });
    }

    const transformed = await readAndTransform(request, env, canonicalPath, url);
    if (!transformed) return new Response("Not found", { status: 404 });
    if (transformed instanceof Response) return transformed; // early-return for 4xx

    const headers = new Headers(transformed.headers);
    headers.set("Cache-Control", isPrivate
      ? "private, no-store"
      : "public, max-age=31536000, immutable");
    return new Response(transformed.body, { status: transformed.status, headers });
  },
};

async function readAndTransform(
  request: Request,
  env: Env,
  canonicalPath: string,
  url: URL,
): Promise<Response | null> {
  const params = parseTransformParams(url);
  if (params instanceof Response) return params; // 400

  const r2Key = canonicalPath.slice(1);
  const obj = await env.R2.get(r2Key);
  if (!obj) return null;

  const fmt = params.fmt === "auto" ? negotiateFormat(request) : `image/${params.fmt}`;
  let pipeline = env.IMAGES.input(obj.body);
  if (params.w !== undefined) pipeline = pipeline.transform({ width: params.w });
  const out = await pipeline.output({ format: fmt, quality: params.q });
  return out.response();
}

function parseTransformParams(url: URL): { w?: number; fmt: string; q: number } | Response {
  const wRaw = url.searchParams.get("w");
  let w: number | undefined;
  if (wRaw !== null) {
    const parsed = parseInt(wRaw, 10);
    if (!isAllowedWidth(parsed)) return new Response("Bad request: w not in allowlist", { status: 400 });
    w = parsed;
  }
  const fmt = url.searchParams.get("fmt") ?? "auto";
  if (!ALLOWED_FORMATS.has(fmt)) return new Response("Bad request: fmt not in allowlist", { status: 400 });
  const qRaw = url.searchParams.get("q");
  const q = qRaw === null ? 85 : parseInt(qRaw, 10);
  if (!ALLOWED_QUALITIES.has(q)) return new Response("Bad request: q not in allowlist", { status: 400 });
  return { w, fmt, q };
}

function negotiateFormat(request: Request): string {
  const accept = request.headers.get("Accept") || "";
  if (accept.includes("image/avif")) return "image/avif";
  if (accept.includes("image/webp")) return "image/webp";
  return "image/jpeg";
}
```

- [ ] **Step 3: Run + commit**

```bash
cd worker && npm test -- handler_public
git add worker/src/index.ts worker/test/handler_public.spec.ts
git commit -m "[worker] feat: public path branch of fetch handler"
```

---

## Task 6: Worker fetch handler — private path

**Files:**
- Modify: `worker/src/index.ts`
- Create: `worker/test/handler_private.spec.ts`
- Create: `worker/test/_signer.ts` (test-only signer)

- [ ] **Step 1: Test-only signer — same algorithm as Plan 1's signer**

```ts
// worker/test/_signer.ts
// In-test ONLY. Never imported from src/. The Worker should not contain
// signing logic — only verification.
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

- [ ] **Step 2: Failing tests** — covers cases 11, 12, 13, 14 from §8.6.1

```ts
// worker/test/handler_private.spec.ts
import { describe, it, expect, beforeEach } from "vitest";
import { env } from "cloudflare:test";
import worker from "../src/index";
import { makeSignedURL } from "./_signer";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

beforeEach(async () => {
  await env.R2.put("private/abc/img1.jpg",
    new Uint8Array([0xff, 0xd8, 0xff /* minimal JPEG */]),
    { httpMetadata: { contentType: "image/jpeg" } });
  // The test environment binds WORKER_SIGNING_KEY to the same hex string.
  // Configure via test setup; see vitest.config.ts.
});

describe("private path", () => {
  it("case 11 — no sig returns 401", async () => {
    const r = await worker.fetch(
      new Request("https://cdn.example.com/img/private/abc/img1.jpg?w=800"), env);
    expect(r.status).toBe(401);
  });

  it("case 12 — valid sig + future exp returns 200 with private/no-store", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, { w: "800" });
    const r = await worker.fetch(new Request(url), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("private, no-store");
  });

  it("rejects expired exp with 401", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) - 60, {});
    const r = await worker.fetch(new Request(url), env);
    expect(r.status).toBe(401);
  });

  it("rejects tampered sig with 401", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, {});
    const tampered = url.replace(/sig=([0-9a-f])/, (m, c) => `sig=${c === "0" ? "1" : "0"}`);
    const r = await worker.fetch(new Request(tampered), env);
    expect(r.status).toBe(401);
  });

  it("case 13 — out-of-allowlist width returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, { w: "99999" });
    const r = await worker.fetch(new Request(url), env);
    expect(r.status).toBe(400);
  });

  it("case 14 — out-of-allowlist fmt returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/abc/img1.jpg", Math.floor(Date.now()/1000) + 300, { fmt: "svg" });
    const r = await worker.fetch(new Request(url), env);
    expect(r.status).toBe(400);
  });
});
```

- [ ] **Step 3: Replace the placeholder private branch in `src/index.ts`**

```ts
// worker/src/index.ts (replace the placeholder)
if (canonicalPath.startsWith("/private/")) {
  isPrivate = true;
  const sig = url.searchParams.get("sig");
  const expStr = url.searchParams.get("exp");
  if (!sig || !expStr) return new Response("Unauthorized", { status: 401 });
  const exp = parseInt(expStr, 10);
  if (!Number.isFinite(exp) || Date.now() / 1000 > exp) {
    return new Response("Unauthorized", { status: 401 });
  }
  const keyBytes = hexToBytes(env.WORKER_SIGNING_KEY);
  const ok = await verifySignature(keyBytes, canonicalPath, exp, sig);
  if (!ok) return new Response("Unauthorized", { status: 401 });
}
```

Add a `hexToBytes` helper:

```ts
function hexToBytes(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.substr(i * 2, 2), 16);
  }
  return out;
}
```

**Important:** the test setup binds `WORKER_SIGNING_KEY` as the **hex encoding** of the key bytes. The Go signer test in Plan 1 uses the literal ASCII string `"0123456789abcdef0123456789abcdef"` (32 ASCII bytes). Hex-encode that for the env var: each ASCII char → 2 hex chars. So the env value becomes `30313233343536373839616263646566303132333435363738396162636465666` (66 chars). Set this in `vitest.config.ts` via `miniflare.bindings`.

Update `vitest.config.ts`:

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
              "30313233343536373839616263646566", // hex of "0123456789abcdef" repeated
          },
          r2Buckets: ["R2"],
          // No real Images binding in Layer A — see Task 7 for the test fake.
        },
      },
    },
  },
});
```

- [ ] **Step 4: Run + commit**

```bash
cd worker && npm test -- handler_private
git add worker/src/index.ts worker/test/handler_private.spec.ts worker/test/_signer.ts worker/vitest.config.ts
git commit -m "[worker] feat: private path with HMAC verify (cases 11-14)"
```

---

## Task 7: Worker fetch handler — IMAGES binding fake + cache headers

**Files:**
- Create: `worker/test/_imagesFake.ts`
- Modify: `worker/vitest.config.ts`
- Create: `worker/test/handler_cache.spec.ts`

The `env.IMAGES` binding is not natively faked by `vitest-pool-workers`. We bind a JS object that conforms to the same shape and asserts the Worker called `transform()` with the expected width.

- [ ] **Step 1: Implement the fake**

```ts
// worker/test/_imagesFake.ts
// Tracks calls so tests can assert the Worker passed the right width/fmt/q.
export type ImagesCall = { width?: number; format: string; quality: number };

export class ImagesFake {
  public calls: ImagesCall[] = [];

  input(stream: ReadableStream): Pipeline {
    return new Pipeline(this, stream);
  }
}

class Pipeline {
  private widthHint?: number;
  constructor(private fake: ImagesFake, private stream: ReadableStream) {}
  transform(opts: { width?: number }): Pipeline {
    this.widthHint = opts.width;
    return this;
  }
  async output(opts: { format: string; quality: number }): Promise<{ response(): Response }> {
    this.fake.calls.push({ width: this.widthHint, format: opts.format, quality: opts.quality });
    // Pass-through bytes; record-only, no real resize.
    return {
      response: () => new Response(this.stream, {
        status: 200,
        headers: { "Content-Type": opts.format },
      }),
    };
  }
}
```

- [ ] **Step 2: Bind the fake in vitest config**

`vitest-pool-workers` doesn't currently let you bind arbitrary JS objects to a Worker via `bindings:`. The cleanest path: **inject the fake at test time** by exposing a setter on the module:

```ts
// worker/src/index.ts (top-level export)
export const _testHooks = {
  imagesOverride: undefined as undefined | { input: (s: ReadableStream) => any },
};
```

And in the handler:

```ts
const images = (typeof globalThis !== "undefined" && (globalThis as any).__IMAGES_OVERRIDE__) ?? env.IMAGES;
```

Or use a wrapper function. The simplest model — and what we'll use — is to make the handler accept `env` and check a side-channel:

```ts
function imagesFor(env: Env): ImagesBinding {
  const override = (globalThis as any).__IMAGES_OVERRIDE__;
  return override ?? env.IMAGES;
}
```

In tests:

```ts
import { ImagesFake } from "./_imagesFake";

beforeEach(() => {
  (globalThis as any).__IMAGES_OVERRIDE__ = new ImagesFake();
});
afterEach(() => {
  delete (globalThis as any).__IMAGES_OVERRIDE__;
});
```

The Layer-B integration test (Task 11) does NOT set this override, so it exercises the real binding.

- [ ] **Step 3: Failing test — verifies Cache-Control + that transform was called**

```ts
// worker/test/handler_cache.spec.ts
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { env } from "cloudflare:test";
import worker from "../src/index";
import { ImagesFake } from "./_imagesFake";

let fake: ImagesFake;

beforeEach(async () => {
  fake = new ImagesFake();
  (globalThis as any).__IMAGES_OVERRIDE__ = fake;
  await env.R2.put("public/abc/img1.jpg", new Uint8Array([0xff, 0xd8]), {
    httpMetadata: { contentType: "image/jpeg" },
  });
});

afterEach(() => { delete (globalThis as any).__IMAGES_OVERRIDE__; });

describe("transform call", () => {
  it("forwards w/fmt/q to IMAGES binding", async () => {
    const r = await worker.fetch(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=800&fmt=webp&q=85"), env);
    expect(r.status).toBe(200);
    expect(fake.calls).toEqual([{ width: 800, format: "image/webp", quality: 85 }]);
    expect(r.headers.get("Cache-Control")).toBe("public, max-age=31536000, immutable");
  });

  it("auto fmt + Accept: avif → image/avif passed through", async () => {
    await worker.fetch(
      new Request("https://cdn.example.com/img/public/abc/img1.jpg?w=240",
        { headers: { Accept: "image/avif,image/*" } }), env);
    expect(fake.calls[0].format).toBe("image/avif");
  });
});
```

- [ ] **Step 4: Wire `imagesFor()` in `src/index.ts`**

Replace the `env.IMAGES` reference in `readAndTransform`:

```ts
let pipeline = imagesFor(env).input(obj.body);
```

- [ ] **Step 5: Run + commit**

```bash
cd worker && npm test -- handler_cache
git add worker/src/index.ts worker/test/_imagesFake.ts worker/test/handler_cache.spec.ts
git commit -m "[worker] feat: IMAGES binding test fake + transform-call assertions"
```

---

## Task 8: Privacy matrix tests (cases 10, 11, 12, 13, 14)

**Files:**
- Create: `worker/test/privacy_matrix.spec.ts`

This consolidates the cases already partially covered in Tasks 5+6 into a single suite that mirrors the design spec's matrix.

- [ ] **Step 1: Test**

```ts
// worker/test/privacy_matrix.spec.ts
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { env } from "cloudflare:test";
import worker from "../src/index";
import { ImagesFake } from "./_imagesFake";
import { makeSignedURL } from "./_signer";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

let fake: ImagesFake;
beforeEach(async () => {
  fake = new ImagesFake();
  (globalThis as any).__IMAGES_OVERRIDE__ = fake;
  await env.R2.put("public/A/p.jpg",  new Uint8Array([0xff, 0xd8]));
  await env.R2.put("private/A/q.jpg", new Uint8Array([0xff, 0xd8]));
});
afterEach(() => { delete (globalThis as any).__IMAGES_OVERRIDE__; });

describe("privacy matrix (Worker rows)", () => {
  it("row 10 — public ?w=800 returns 200, immutable cache", async () => {
    const r = await worker.fetch(
      new Request("https://cdn.example.com/img/public/A/p.jpg?w=800"), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("public, max-age=31536000, immutable");
    expect(fake.calls[0].width).toBe(800);
  });

  it("row 11 — private without sig returns 401", async () => {
    const r = await worker.fetch(
      new Request("https://cdn.example.com/img/private/A/q.jpg?w=800"), env);
    expect(r.status).toBe(401);
  });

  it("row 12 — private with valid sig returns 200, no-store", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/A/q.jpg", Math.floor(Date.now()/1000) + 300, { w: "800" });
    const r = await worker.fetch(new Request(url), env);
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("private, no-store");
    expect(fake.calls[0].width).toBe(800);
  });

  it("row 13 — w=99999 returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/A/q.jpg", Math.floor(Date.now()/1000) + 300, { w: "99999" });
    const r = await worker.fetch(new Request(url), env);
    expect(r.status).toBe(400);
  });

  it("row 14 — fmt=svg returns 400", async () => {
    const url = await makeSignedURL("https://cdn.example.com", KEY,
      "private/A/q.jpg", Math.floor(Date.now()/1000) + 300, { fmt: "svg" });
    const r = await worker.fetch(new Request(url), env);
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
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { env } from "cloudflare:test";
import worker from "../src/index";
import { ImagesFake } from "./_imagesFake";
import { makeSignedURL } from "./_signer";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

beforeEach(async () => {
  (globalThis as any).__IMAGES_OVERRIDE__ = new ImagesFake();
  await env.R2.put("private/X/img.jpg", new Uint8Array([0xff, 0xd8]));
});
afterEach(() => { delete (globalThis as any).__IMAGES_OVERRIDE__; });

describe("case 16 — cache-key safety", () => {
  it("owner-then-anonymous: anon never receives owner bytes", async () => {
    // Owner request with valid sig.
    const ownerURL = await makeSignedURL("https://cdn.example.com", KEY,
      "private/X/img.jpg", Math.floor(Date.now()/1000) + 300, { w: "800" });
    const ownerResp = await worker.fetch(new Request(ownerURL), env);
    expect(ownerResp.status).toBe(200);
    expect(ownerResp.headers.get("Cache-Control")).toBe("private, no-store");

    // Anonymous request to the same path with no sig.
    const anonResp = await worker.fetch(
      new Request("https://cdn.example.com/img/private/X/img.jpg?w=800"), env);
    expect(anonResp.status).toBe(401);
  });

  it("owner-then-tampered: tampered sig never receives owner bytes", async () => {
    const ownerURL = await makeSignedURL("https://cdn.example.com", KEY,
      "private/X/img.jpg", Math.floor(Date.now()/1000) + 300, {});
    const ownerResp = await worker.fetch(new Request(ownerURL), env);
    expect(ownerResp.status).toBe(200);

    const tampered = ownerURL.replace(/sig=([0-9a-f])/, (_, c) =>
      "sig=" + (c === "0" ? "1" : "0"));
    const r = await worker.fetch(new Request(tampered), env);
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
    expect(got).toBe("<PASTE-FROM-PLAN-1-TASK-8>");
  });
});
```

- [ ] **Step 2: Run** — should pass once the placeholder is replaced with the actual hex from Plan 1 Task 8.

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

    const fd = new FormData();
    fd.set("manifest", JSON.stringify([
      { client_image_id: "K", position: 0, content_type: "image/jpeg" },
    ]));
    // 2400-px wide JPEG fixture, ~30 KB; committed under test_integration/fixtures/
    const file = await fetch("file://" + import.meta.dir + "/fixtures/2400px.jpg")
      .then(r => r.blob());
    fd.set("files", file, "2400.jpg");
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

Generate `test_integration/fixtures/2400px.jpg` with any 2400×N JPEG (a solid-color fixture is fine). One-shot:

```bash
cd worker && mkdir -p test_integration/fixtures
node -e "
const f=require('fs');
const w=2400,h=1600;
// Pure-JS minimal JPEG generator is large; instead, use a fixture from a
// prebuilt small JPEG and rely on the API to accept it. For initial wiring,
// drop in any real JPEG of the right dimensions.
"
# In practice, use ImageMagick: `convert -size 2400x1600 xc:steelblue 2400px.jpg`
```

Commit the binary fixture (a few KB). Document the convert command in `test_integration/README.md`.

- [ ] **Step 4: README documenting how to run Layer B locally**

```markdown
# Layer-B integration tests

Runs the full round-trip described in spec §8.6.2 case 15.

**Prerequisites:**

1. Plan 1's API running locally: `cd ../api && go run ./cmd/api` with `WORKER_SIGNING_KEY`, `JWT_SIGNING_KEY`, `DATABASE_URL`, `CDN_ORIGIN=http://localhost:8787` exported.
2. This Worker running locally: `cd .. && npm run dev` (wrangler dev). Same `WORKER_SIGNING_KEY`. R2 bound to a dev bucket; `[images]` binding requires a Cloudflare account in `wrangler.toml`'s `[dev]` block — set `experimental_remote = true` for the IMAGES binding to use real Cloudflare image transforms in dev.
3. An auth cookie / JWT for an owner user. Easiest: hit `GET /auth/google/start` in a browser, then copy the `auth` cookie value.

**Run:**

```bash
ART_API_BASE=http://localhost:8080 \
ART_CDN_BASE=http://localhost:8787 \
ART_OWNER_JWT=<paste-cookie-value> \
npm run test:integration
```
```

- [ ] **Step 5: Commit**

```bash
git add worker/vitest.integration.config.ts worker/test_integration/
git commit -m "[worker] test: layer-B round-trip with image-size width verification (case 15)"
```

---

## Task 12: `.github/workflows/worker.yml`

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

After all 12 tasks land, Plan 2 produces a Cloudflare Worker that:

- Validates canonical paths (contracts §3) and rejects malformed input with 400
- Verifies HMAC signatures byte-perfect against Plan 1 (via the locked vector pin in Task 10)
- Reads R2 via binding, transforms via `env.IMAGES` with the allowlist (contracts §6), emits the right Cache-Control per visibility
- Passes privacy-matrix rows 10–14 (§8.6.1) and the cache-safety regression (§8.6.2 case 16) in CI without Cloudflare credentials
- Has a Layer-B round-trip test (case 15) ready to run against `wrangler dev` + the API

The Worker source is ~120 lines as the spec mandates. Anything that grows beyond that is a refactor signal.
