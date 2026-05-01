// web/lib/cf-loader.ts
const ALLOWED_WIDTHS = [240, 480, 800, 1024, 1600, 2400] as const;
const ALLOWED_QUALITIES = new Set([60, 75, 85, 90]);

function pickWidth(requested: number): number {
  for (const w of ALLOWED_WIDTHS) if (w >= requested) return w;
  return ALLOWED_WIDTHS[ALLOWED_WIDTHS.length - 1];
}

export default function cfLoader(args: { src: string; width: number; quality?: number }): string {
  const u = new URL(args.src);
  const publicOrigin = process.env.NEXT_PUBLIC_CDN_BASE;
  if (publicOrigin) {
    const origin = new URL(publicOrigin);
    u.protocol = origin.protocol;
    u.host = origin.host;
  }
  u.searchParams.set("w", String(pickWidth(args.width)));
  u.searchParams.set("fmt", "auto");
  const q = args.quality && ALLOWED_QUALITIES.has(args.quality) ? args.quality : 85;
  u.searchParams.set("q", String(q));
  return u.toString();
}
