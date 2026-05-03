import { loadEnvConfig } from "@next/env";
import { defineConfig } from "@playwright/test";

loadEnvConfig(process.cwd());

const isHeaded = process.env.PW_HEADED === "true";
const slowMo = Number(process.env.PW_SLOW ?? "500");

export default defineConfig({
  testDir: "./e2e",
  // Runs once before the suite to verify the API/CDN/web endpoints are up.
  globalSetup: "./e2e/global-setup",
  use: {
    // `page.goto("/")` in tests resolves against this app server.
    baseURL: process.env.WEB_BASE || "http://localhost:3000",
    headless: !isHeaded,
    launchOptions: isHeaded ? { slowMo: Number.isFinite(slowMo) ? slowMo : 500 } : {},
    extraHTTPHeaders: {},
  },
  projects: [
    // Playwright will launch a real Chromium browser for this project.
    { name: "chromium", use: { browserName: "chromium" } },
  ],
  reporter: [["list"], ["html", { open: "never" }]],
});
