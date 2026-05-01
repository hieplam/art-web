// worker/test/path.spec.ts
import { describe, it, expect } from "vitest";
import { validateCanonicalPath } from "../src/path";

describe("validateCanonicalPath", () => {
  const good = [
    "/private/8f3a4b2c-7e6d-4f1a-9c3e-2b5a8d7c1e9f/0a1b2c3d-4e5f-6789-abcd-ef0123456789.jpg",
    "/public/aaa/bbb.png",
  ];
  for (const p of good) {
    it(`accepts ${p}`, () => expect(validateCanonicalPath(p)).toBeNull());
  }

  const bad = [
    ["", "empty"],
    ["private/x.jpg", "no leading slash"],
    ["/private/x.jpg/", "trailing slash"],
    ["/private//x.jpg", "double slash"],
    ["/private/../etc.jpg", "dot-dot"],
    ["/private/./x.jpg", "dot"],
    ["/private/abc def.jpg", "space"],
    ["/private/abc%20.jpg", "percent"],
    ["/private/abc?q=1.jpg", "querychar"],
  ];
  for (const [p, why] of bad) {
    it(`rejects ${p} (${why})`, () => expect(validateCanonicalPath(p)).not.toBeNull());
  }
});
