// worker/vitest.config.ts
import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    poolOptions: {
      workers: {
        wrangler: { configPath: "./wrangler.toml" },
        miniflare: {
          r2Buckets: ["R2"],
          // IMAGES is intentionally not bound — Layer-A tests inject a
          // fake via `mkTestEnv()` (Task 5). Layer-B (Task 11) hits the
          // real binding under `wrangler dev`.
        },
      },
    },
  },
});
