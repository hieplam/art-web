"use client";
import { useEffect, useRef, useState } from "react";

type Page<T> = { items: T[]; next_cursor: string | null };

type Props<T extends { id: string }> = {
  initialItems: T[];
  initialCursor: string | null;
  fetchMore: (cursor: string) => Promise<Page<T>>;
  renderItem: (item: T) => JSX.Element;
};

export function InfiniteFeed<T extends { id: string }>(props: Props<T>) {
  const [items, setItems] = useState(props.initialItems);
  const [cursor, setCursor] = useState(props.initialCursor);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef(false);

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !cursor) return;
    const obs = new IntersectionObserver(async ([entry]) => {
      if (!entry.isIntersecting || loadingRef.current) return;
      loadingRef.current = true;
      try {
        const page = await props.fetchMore(cursor);
        setItems((prev) => {
          const seen = new Set(prev.map((p) => p.id));
          return [...prev, ...page.items.filter((i) => !seen.has(i.id))];
        });
        setCursor(page.next_cursor);
      } finally {
        loadingRef.current = false;
      }
    });
    obs.observe(el);
    return () => obs.disconnect();
  }, [cursor]);

  return (
    <>
      {items.map(props.renderItem)}
      {cursor && <div ref={sentinelRef} aria-hidden style={{ height: 1 }} />}
    </>
  );
}
