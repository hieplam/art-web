// worker/vitest.integration.config.ts
import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["test_integration/**/*.spec.ts"],
    environment: "node",
    testTimeout: 30000,
  },
});
