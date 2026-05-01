// worker/test/allowlist.spec.ts
import { describe, it, expect } from "vitest";
import { ALLOWED_WIDTHS, ALLOWED_FORMATS, ALLOWED_QUALITIES, isAllowedWidth } from "../src/allowlist";

describe("allowlist", () => {
  it("matches contracts §6 widths exactly", () => {
    expect([...ALLOWED_WIDTHS].sort((a, b) => a - b))
      .toEqual([240, 480, 800, 1024, 1600, 2400]);
  });
  it("matches contracts §6 formats exactly", () => {
    expect([...ALLOWED_FORMATS].sort()).toEqual(["auto", "avif", "jpeg", "webp"]);
  });
  it("matches contracts §6 qualities exactly", () => {
    expect([...ALLOWED_QUALITIES].sort((a, b) => a - b)).toEqual([60, 75, 85, 90]);
  });
  it("isAllowedWidth rejects out-of-set", () => {
    expect(isAllowedWidth(800)).toBe(true);
    expect(isAllowedWidth(801)).toBe(false);
    expect(isAllowedWidth(99999)).toBe(false);
  });
});
