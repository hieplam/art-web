// worker/test/sign.spec.ts
import { describe, it, expect } from "vitest";
import { verifySignature } from "../src/sign";

// The Go signer test in Plan 1 uses the literal ASCII string
// "0123456789abcdef0123456789abcdef" as the key (32 bytes). We use the
// same bytes here so the locked HMAC vector matches.
const TEST_KEY_BYTES = new TextEncoder().encode("0123456789abcdef0123456789abcdef");

describe("verifySignature", () => {
  it("accepts the locked vector from Plan 1 Task 8", async () => {
    // Paste the hex produced by the Go test; this is the contract anchor.
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000000, expected);
    expect(ok).toBe(true);
  });

  it("rejects a tampered signature", async () => {
    // Flip one nibble of a known-good signature; expect rejection.
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const tampered = expected.slice(0, -1) + (expected.slice(-1) === "0" ? "1" : "0");
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000000, tampered);
    expect(ok).toBe(false);
  });

  it("rejects a different canonical path", async () => {
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/ccc.jpg", 1700000000, expected);
    expect(ok).toBe(false);
  });

  it("rejects a different exp", async () => {
    const expected = "c018a64c183bc6d6348dbf8e2780d287e3550b6945037fa4bedf05f5aa663654";
    const ok = await verifySignature(TEST_KEY_BYTES, "/private/aaa/bbb.jpg", 1700000001, expected);
    expect(ok).toBe(false);
  });
});
