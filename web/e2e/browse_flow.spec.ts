import { test, expect } from "@playwright/test";
import { seedMatrix } from "./_helpers";

function sampleRandom<T>(items: T[], count: number): T[] {
  const pool = [...items];
  for (let i = pool.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }
  return pool.slice(0, count);
}

test("full flow — browse 10 random seeded artworks from home to detail and back", async ({ page }) => {
  await seedMatrix({ many: 40 });

  await page.goto("/");
  await page.waitForLoadState("networkidle");

  const artworkIds = await page.$$eval("[data-artwork-id]", (els) =>
    els
      .map((el) => el.getAttribute("data-artwork-id"))
      .filter((id): id is string => Boolean(id))
  );
  expect(artworkIds.length).toBeGreaterThanOrEqual(10);

  const randomIds = sampleRandom(artworkIds, 10);

  for (const artworkId of randomIds) {
    await page.locator(`[data-artwork-id="${artworkId}"]`).click();
    await expect(page).toHaveURL(new RegExp(`/art/${artworkId}$`));
    await expect(page.locator("article.detail")).toBeVisible();
    await expect(page.locator(".detail-title")).toBeVisible();
    await expect(page.locator(".detail-image").first()).toBeVisible();

    await page.goBack();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.locator(`[data-artwork-id="${artworkId}"]`)).toBeVisible();
  }
});
