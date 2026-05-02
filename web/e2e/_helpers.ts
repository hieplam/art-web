// These helpers talk directly to the API to set up the matrix:
//   user A (alice) owns artwork P (public, tag "t") and artwork Q (private, tag "t")
//   user B (bob) is another signed-in user
//   anon = no cookie
//
// The /dev/seed endpoint is registered by Plan 1 Task 33, gated by
// APP_ENV=test. Outside test env it returns 404 (the route is not
// registered AND the handler re-checks). The API in web/docker-compose.e2e.yml
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
