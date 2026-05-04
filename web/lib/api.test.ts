// web/lib/api.test.ts
import { describe, it, expect, vi } from "vitest";
import { api, ApiClientError, isUnauthenticatedError } from "./api";

describe("api client", () => {
  it("forwards cookie header when given", async () => {
    const fetchSpy = vi.fn().mockResolvedValue(new Response(`{"ok":1}`, { status: 200 }));
    vi.stubGlobal("fetch", fetchSpy);
    await api({ base: "http://api", path: "/me", cookie: "auth=abc" });
    expect(fetchSpy).toHaveBeenCalledWith("http://api/me", expect.objectContaining({
      headers: expect.objectContaining({ Cookie: "auth=abc" }),
      cache: "no-store",
    }));
  });

  it("throws ApiClientError with status + body", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(`{"error":"not_found"}`, { status: 404 })));
    await expect(api({ base: "http://api", path: "/nope" })).rejects.toMatchObject({
      status: 404,
      body: { error: "not_found" },
    });
  });

  it("returns parsed JSON for 2xx", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response(`{"hi":"world"}`, { status: 200 })));
    const out = await api({ base: "http://api", path: "/x" });
    expect(out).toEqual({ hi: "world" });
  });
});

describe("isUnauthenticatedError", () => {
  // Treats 401 and 404 the same so server components stay anonymous on
  // either an invalid JWT (current API contract) or a stale-session 404
  // from older API builds where /me was returning not_found.
  it("matches a 401 ApiClientError", () => {
    expect(isUnauthenticatedError(new ApiClientError(401, { error: "unauthorized" })))
      .toBe(true);
  });

  it("matches a 404 ApiClientError (stale session compatibility)", () => {
    expect(isUnauthenticatedError(new ApiClientError(404, { error: "not_found" })))
      .toBe(true);
  });

  it("does not match other status codes", () => {
    expect(isUnauthenticatedError(new ApiClientError(500, { error: "boom" }))).toBe(false);
    expect(isUnauthenticatedError(new ApiClientError(403, { error: "forbidden" }))).toBe(false);
  });

  it("does not match plain Errors or non-Error values", () => {
    expect(isUnauthenticatedError(new Error("network down"))).toBe(false);
    expect(isUnauthenticatedError("oops")).toBe(false);
    expect(isUnauthenticatedError(undefined)).toBe(false);
    expect(isUnauthenticatedError(null)).toBe(false);
  });
});
