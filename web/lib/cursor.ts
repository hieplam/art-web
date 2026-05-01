// web/lib/cursor.ts
export function appendCursor(path: string, cursor: string | null): string {
  if (!cursor) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}cursor=${encodeURIComponent(cursor)}`;
}
