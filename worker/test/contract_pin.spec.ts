// worker/test/contract_pin.spec.ts
import { describe, it, expect } from "vitest";
import { computeSignature } from "../src/sign";

const KEY = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

describe("cross-language signature contract", () => {
  it("matches Plan 1 Task 8 fixed vector for /private/aaa/bbb.jpg @ 1700000000", async () => {
    const got = await computeSignature(KEY, "/private/aaa/bbb.jpg", 1700000000);
    expect(got).toBe("c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654");
  });
});
