// @vitest-environment jsdom
// web/components/ArtCard.test.tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ArtCard } from "./ArtCard";

const item = {
  id: "art-1", title: "Hello", visibility: "public" as const,
  published_at: "2026-04-30T00:00:00Z", created_at: "2026-04-30T00:00:00Z",
  cover: { id: "i-1", url: "https://cdn.example.com/img/public/a/b.jpg",
           width: 800, height: 600, blurhash: "L0", position: 0 },
  artist: { id: "u-1", display_name: "Alice", slug: "alice", avatar_url: null },
};

describe("ArtCard", () => {
  it("renders link to /art/:id with the artwork id as data attribute", () => {
    render(<ArtCard item={item} />);
    const link = screen.getByRole("link");
    expect(link.getAttribute("href")).toBe("/art/art-1");
    expect(link.getAttribute("data-artwork-id")).toBe("art-1");
  });

  it("includes width and height to reserve aspect-ratio space", () => {
    render(<ArtCard item={item} />);
    const img = screen.getByRole("img");
    expect(img.getAttribute("width")).toBe("800");
    expect(img.getAttribute("height")).toBe("600");
  });
});
