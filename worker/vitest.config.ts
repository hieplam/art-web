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
        // Disable per-test storage isolation: the WAL-mode SQLite files
        // created by miniflare's R2 include .sqlite-shm/.sqlite-wal
        // auxiliary files that trip up the stack-frame snapshot check in
        // @cloudflare/vitest-pool-workers 0.5.x. Tests that mutate R2
        // reset state explicitly in beforeEach() instead.
        isolatedStorage: false,
      },
    },
  },
});
