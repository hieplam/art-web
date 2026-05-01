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
