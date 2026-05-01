// web/e2e/ux.spec.ts
import { test, expect } from "@playwright/test";
import { seedMatrix } from "./_helpers";

let env: Awaited<ReturnType<typeof seedMatrix>>;
test.beforeAll(async () => { env = await seedMatrix(); });

test("case 22 — infinite scroll does not duplicate items", async ({ page }) => {
  await seedMatrix({ many: 120 });
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
  page.on("request", (r) => {
    if (r.url().includes("/img/")) requested.push(r.url());
  });
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  const initialCount = requested.length;
  await page.evaluate(() => window.scrollBy(0, window.innerHeight * 3));
  await page.waitForLoadState("networkidle");
  expect(requested.length).toBeGreaterThan(initialCount);
});

test("case 25 — flip private + incognito → disappearance", async ({ browser }) => {
  const incog = await browser.newContext();
  const ip = await incog.newPage();
  await ip.goto(`/u/${env.aliceSlug}`);
  expect(await ip.content()).toContain(env.pId);

  await fetch(`${process.env.API_BASE}/artworks/${env.pId}`, {
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
