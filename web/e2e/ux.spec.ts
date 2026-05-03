// web/e2e/ux.spec.ts
import { test, expect } from "@playwright/test";
import { seedMatrix } from "./_helpers";

let env: Awaited<ReturnType<typeof seedMatrix>>;
// Seed shared fixture IDs/cookies once for this file. Each individual test still
// gets its own isolated Playwright browser context unless it creates extras.
test.beforeAll(async () => { env = await seedMatrix(); });

test("case 22 — infinite scroll does not duplicate items", async ({ page }) => {
  await seedMatrix({ many: 120 });
  // `page` is one real browser tab created by Playwright for this test.
  await page.goto("/");
  for (let i = 0; i < 5; i++) {
    await page.evaluate(() => window.scrollBy(0, document.body.scrollHeight));
    await page.waitForTimeout(500);
  }
  const ids = await page.$$eval("[data-artwork-id]", (els) =>
    els.map((e) => e.getAttribute("data-artwork-id"))
  );
  expect(ids.length).toBeGreaterThanOrEqual(120);
  expect(new Set(ids).size).toBe(ids.length);
});

test("case 23 — masonry reserves space; no layout shift > 0.05", async ({ page }) => {
  await page.goto("/");
  const start = await page.$$eval("[data-artwork-id]", els =>
    els.map(e => e.getBoundingClientRect()).slice(0, 8).map(b => ({ y: b.top, h: b.height })));
  await page.waitForLoadState("networkidle");
  const end = await page.$$eval("[data-artwork-id]", els =>
    els.map(e => e.getBoundingClientRect()).slice(0, 8).map(b => ({ y: b.top, h: b.height })));
  for (let i = 0; i < start.length; i++) {
    expect(Math.abs(start[i].y - end[i].y)).toBeLessThan(2);
    expect(Math.abs(start[i].h - end[i].h)).toBeLessThan(2);
  }
});

test("case 24 — lazy loading: only near-viewport images fetched initially", async ({ page }) => {
  const requested: string[] = [];
  // Observe browser-side network traffic as the page lazily fetches images.
  page.on("request", (r) => {
    if (r.url().includes("/img/")) requested.push(r.url());
  });
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  const initialCount = requested.length;
  await page.evaluate(() => window.scrollBy(0, window.innerHeight * 3));
  await expect.poll(() => requested.length, { timeout: 5_000 }).toBeGreaterThan(initialCount);
});

test("case 25 — flip private + incognito → disappearance", async ({ browser }) => {
  // Create a second isolated session, like an incognito profile, so we can
  // verify what an anonymous visitor sees independently of any signed-in state.
  const incog = await browser.newContext();
  const ip = await incog.newPage();
  await ip.goto(`/u/${env.aliceSlug}`);
  expect(await ip.content()).toContain(env.pId);

  const apiBase = process.env.API_BASE || "http://localhost:8080";
  await fetch(`${apiBase}/artworks/${env.pId}`, {
    method: "PATCH",
    headers: { Cookie: env.aliceCookie, "Content-Type": "application/json" },
    body: JSON.stringify({ visibility: "private" }),
  });

  await expect.poll(async () => {
    await ip.reload();
    return (await ip.content()).includes(env.pId);
  }, { timeout: 60_000, intervals: [500, 1000, 2000, 4000] }).toBe(false);

  await ip.goto("/");
  expect(await ip.content()).not.toContain(env.pId);
  await ip.goto(`/tag/t`);
  expect(await ip.content()).not.toContain(env.pId);
  const directHTML = await (await ip.request.get(`/art/${env.pId}`)).text();
  expect(directHTML).not.toContain(env.pId);
  await incog.close();
});
