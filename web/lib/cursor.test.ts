// web/lib/cursor.test.ts
import { describe, it, expect } from "vitest";
import { appendCursor } from "./cursor";

describe("appendCursor", () => {
  it("appends with ? when none present", () => {
    expect(appendCursor("/x", "abc")).toBe("/x?cursor=abc");
  });
  it("appends with & when ? present", () => {
    expect(appendCursor("/x?y=1", "abc")).toBe("/x?y=1&cursor=abc");
  });
  it("URL-encodes special chars", () => {
    expect(appendCursor("/x", "a/b=c")).toBe("/x?cursor=a%2Fb%3Dc");
  });
  it("returns path unchanged when cursor is null", () => {
    expect(appendCursor("/x?y=1", null)).toBe("/x?y=1");
  });
});
