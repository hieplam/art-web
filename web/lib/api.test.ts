// web/lib/api.test.ts
import { describe, it, expect, vi } from "vitest";
import { api, ApiClientError } from "./api";

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
