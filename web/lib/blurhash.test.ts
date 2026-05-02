// web/lib/blurhash.test.ts
import { describe, it, expect } from "vitest";
import { blurhashToDataURL } from "./blurhash";

describe("blurhashToDataURL", () => {
  it("returns a base64 PNG data URL", () => {
    const url = blurhashToDataURL("L6PZfSi_.AyE_3t7t7R**0o#DgR4");
    expect(url.startsWith("data:image/png;base64,")).toBe(true);
    expect(url.length).toBeGreaterThan(120);
  });

  it("returns a 1x1 transparent fallback for empty input", () => {
    const url = blurhashToDataURL("");
    expect(url).toBe("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=");
  });
});
