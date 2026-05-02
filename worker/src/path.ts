// worker/src/path.ts
const ALLOWED_RE = /^[A-Za-z0-9/._-]+$/;

export function validateCanonicalPath(p: string): string | null {
  if (!p || p[0] !== "/") return "must start with /";
  if (p.endsWith("/")) return "must not end with /";
  if (p.includes("//") || p.includes("/../") || p.includes("/./"))
    return "contains forbidden segment";
  if (p.endsWith("/..") || p.endsWith("/.")) return "contains forbidden segment";
  if (!ALLOWED_RE.test(p)) return "char not allowed";
  return null;
}
