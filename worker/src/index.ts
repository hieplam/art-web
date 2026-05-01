// worker/src/index.ts
import { ALLOWED_FORMATS, ALLOWED_QUALITIES, isAllowedWidth } from "./allowlist";
import { validateCanonicalPath } from "./path";
import { verifySignature } from "./sign"; // used in Task 6 private-path HMAC verification

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

  // Detect canonicalization attacks: new URL() normalises dot-segments,
  // so "img/public/../secret.jpg" becomes "img/secret.jpg". Compare the
  // raw path from the request string against the normalised pathname;
  // if they differ the client sent a non-canonical path — reject with 400.
  const rawPath = request.url.split("?")[0].slice(url.origin.length);
  if (rawPath !== url.pathname) {
    return new Response("Bad request: non-canonical path", { status: 400 });
  }

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
    // Path is inside /img/ but not under a known namespace (/public/ or
    // /private/). This includes paths that were canonicalized from
    // dot-segment traversals (e.g. /img/public/../foo.jpg → /img/foo.jpg).
    // Treat as a bad request so callers get a clear signal rather than a
    // misleading 404.
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
