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
