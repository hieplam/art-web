import { chromium } from "@playwright/test";

export default async function globalSetup() {
  const api = process.env.API_BASE || "http://localhost:8080";
  const cdn = process.env.CDN_BASE || "http://localhost:8787";
  const web = process.env.WEB_BASE || "http://localhost:3000";

  for (const [label, url] of [["API", api], ["CDN", cdn], ["WEB", web]]) {
    const r = await fetch(`${url}/`, { method: "GET" });
    if (!r.ok && r.status !== 404) throw new Error(`${label} not reachable: ${r.status}`);
  }

  const browser = await chromium.launch();
  await browser.close();
}
