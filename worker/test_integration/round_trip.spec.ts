// worker/test_integration/round_trip.spec.ts
import { describe, it, expect } from "vitest";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import imageSize from "image-size";

const API   = process.env.ART_API_BASE  || "http://localhost:8080";
const CDN   = process.env.ART_CDN_BASE  || "http://localhost:8787";
const TOKEN = process.env.ART_OWNER_JWT || "";

describe.skipIf(!TOKEN)("case 15 — signed-URL round-trip", () => {
  it("API-issued private URL renders 200 + correct width", async () => {
    // 1. Owner creates a private artwork and uploads one image.
    const created = await fetch(`${API}/artworks`, {
      method: "POST",
      headers: { Cookie: `auth=${TOKEN}`, "Content-Type": "application/json" },
      body: JSON.stringify({ title: "rt", visibility: "private" }),
    });
    const art = await created.json() as any;

    const fixturePath = fileURLToPath(new URL("./fixtures/2400px.jpg", import.meta.url));
    const fixtureBytes = await readFile(fixturePath);

    const fd = new FormData();
    fd.set("manifest", JSON.stringify([
      { client_image_id: "K", position: 0, content_type: "image/jpeg" },
    ]));
    fd.set("files", new Blob([fixtureBytes], { type: "image/jpeg" }), "2400.jpg");
    await fetch(`${API}/artworks/${art.id}/images`, {
      method: "POST", headers: { Cookie: `auth=${TOKEN}` }, body: fd,
    });

    // 2. Fetch detail, extract the signed URL, swap CDN base if needed.
    const detail = await fetch(`${API}/artworks/${art.id}`, {
      headers: { Cookie: `auth=${TOKEN}` },
    }).then(r => r.json() as any);
    const signed = detail.images[0].url
      .replace(/^https?:\/\/cdn\.example\.com/, CDN);

    // 3. GET signed?w=800 against the Worker; decode the bytes.
    const u = new URL(signed);
    u.searchParams.set("w", "800");
    u.searchParams.set("fmt", "jpeg");
    u.searchParams.set("q", "85");
    const resp = await fetch(u);
    expect(resp.status).toBe(200);
    const buf = Buffer.from(await resp.arrayBuffer());
    const dims = imageSize(buf);
    expect(dims.width).toBeGreaterThanOrEqual(799);
    expect(dims.width).toBeLessThanOrEqual(801);
  });
});
