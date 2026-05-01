import { render, screen, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InfiniteFeed } from "./InfiniteFeed";

let triggerEnter: () => void;

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", class {
    constructor(cb: (entries: { isIntersecting: boolean }[]) => void) {
      triggerEnter = () => cb([{ isIntersecting: true }]);
    }
    observe() {}
    disconnect() {}
  });
});

describe("InfiniteFeed", () => {
  it("calls fetchMore when sentinel intersects, then renders new items", async () => {
    const fetchMore = vi.fn().mockResolvedValueOnce({
      items: [{ id: "x" }], next_cursor: null,
    });
    render(<InfiniteFeed
      initialItems={[{ id: "a" }]}
      initialCursor="c1"
      fetchMore={fetchMore}
      renderItem={(item) => <div data-testid="row">{item.id}</div>}
    />);

    expect(screen.getAllByTestId("row")).toHaveLength(1);
    await act(async () => { triggerEnter(); });
    expect(fetchMore).toHaveBeenCalledWith("c1");
    expect(screen.getAllByTestId("row")).toHaveLength(2);
  });

  it("does not duplicate items if fetchMore returns an item already shown", async () => {
    const fetchMore = vi.fn().mockResolvedValueOnce({
      items: [{ id: "a" }, { id: "b" }], next_cursor: null,
    });
    render(<InfiniteFeed
      initialItems={[{ id: "a" }]}
      initialCursor="c1"
      fetchMore={fetchMore}
      renderItem={(item) => <div data-testid="row" data-id={item.id} />}
    />);
    await act(async () => { triggerEnter(); });
    const ids = Array.from(document.querySelectorAll("[data-id]")).map(e => e.getAttribute("data-id"));
    expect(ids).toEqual(["a", "b"]); // 'a' deduplicated
  });
});
