// worker/src/allowlist.ts
// Mirrors contracts §6 byte-for-byte. Any change here must also land
// in web/lib/cf-loader.ts AND web/next.config.js deviceSizes.
export const ALLOWED_WIDTHS    = new Set<number>([240, 480, 800, 1024, 1600, 2400]);
export const ALLOWED_FORMATS   = new Set<string>(["auto", "avif", "webp", "jpeg"]);
export const ALLOWED_QUALITIES = new Set<number>([60, 75, 85, 90]);

export function isAllowedWidth(w: number): boolean {
  return Number.isFinite(w) && ALLOWED_WIDTHS.has(w);
}
