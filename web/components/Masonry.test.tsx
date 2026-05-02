import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Masonry } from "./Masonry";

describe("Masonry", () => {
  it("applies the masonry class so children flow into columns", () => {
    const { container } = render(<Masonry>{[<div key="a" />]}</Masonry>);
    expect(container.firstElementChild?.className).toContain("masonry");
  });
});
