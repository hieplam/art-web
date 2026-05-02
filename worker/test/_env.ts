// worker/test/_env.ts
// `wpwEnv` is the worker's bindings as configured by `vitest.config.ts` —
// real workerd-bound R2 and WORKER_SIGNING_KEY. We spread it so tests get
// the real R2 (so .put / .get round-trip) and override IMAGES with a fake.
import { env as wpwEnv } from "cloudflare:test";
import { ImagesFake } from "./_imagesFake";
import type { Env } from "../src/index";

export type TestEnv = Env & { _images: ImagesFake };

export function mkTestEnv(): TestEnv {
  const fake = new ImagesFake();
  return {
    ...(wpwEnv as unknown as Env),
    IMAGES: fake as unknown as Env["IMAGES"],
    _images: fake,
  };
}
