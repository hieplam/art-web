// web/lib/cf-loader.test.ts
import { describe, it, expect, vi } from "vitest";
import cfLoader from "./cf-loader";

describe("cf-loader", () => {
  it("clamps to nearest-up allowlist width", () => {
    const out = cfLoader({ src: "https://cdn.example.com/img/public/a/b.jpg", width: 700 });
    expect(out).toContain("w=800");
  });

  it("falls back to largest when oversize", () => {
    const out = cfLoader({ src: "https://cdn.example.com/img/public/a/b.jpg", width: 9999 });
    expect(out).toContain("w=2400");
  });

  it("preserves sig + exp on private URLs", () => {
    const out = cfLoader({
      src: "https://cdn.example.com/img/private/a/b.jpg?sig=ABC&exp=123",
      width: 240, quality: 90,
    });
    const u = new URL(out);
    expect(u.searchParams.get("sig")).toBe("ABC");
    expect(u.searchParams.get("exp")).toBe("123");
    expect(u.searchParams.get("w")).toBe("240");
    expect(u.searchParams.get("q")).toBe("90");
    expect(u.searchParams.get("fmt")).toBe("auto");
  });

  it("defaults q=85 when not specified", () => {
    const u = new URL(cfLoader({ src: "https://cdn.example.com/img/public/x.jpg", width: 800 }));
    expect(u.searchParams.get("q")).toBe("85");
  });

  it("rewrites container CDN origins for the browser when configured", () => {
    vi.stubEnv("NEXT_PUBLIC_CDN_BASE", "http://localhost:8787");
    const u = new URL(cfLoader({ src: "http://worker:8787/img/public/x.jpg", width: 800 }));
    expect(u.origin).toBe("http://localhost:8787");
    vi.unstubAllEnvs();
  });
});
