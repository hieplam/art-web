// worker/vitest.config.ts
import { defineWorkersConfig } from "@cloudflare/vitest-pool-workers/config";

export default defineWorkersConfig({
  test: {
    poolOptions: {
      workers: {
        // Use the test-specific config: wrangler.toml's [images] section is
        // not recognised by the bundled miniflare version and crashes workerd.
        wrangler: { configPath: "./wrangler.test.toml" },
        miniflare: {
          bindings: {
            // ASCII "0123456789abcdef" repeated twice → 64 hex chars.
            // Matches the key used in Plan 1 Task 8 Go signer tests.
            WORKER_SIGNING_KEY:
              "30313233343536373839616263646566" +
              "30313233343536373839616263646566",
          },
          // R2 comes from wrangler.test.toml — do NOT add r2Buckets here
          // or workerd crashes with "inserted row already exists in table".
          // IMAGES is intentionally not bound — Layer-A tests inject a
          // fake via `mkTestEnv()` (Task 5). Layer-B (Task 11) hits the
          // real binding under `wrangler dev`.
        },
        isolatedStorage: false,
      },
    },
  },
});
