"use client";
import React, { useEffect, useRef, useState } from "react";

type Page<T> = { items: T[]; next_cursor: string | null };

type Props<T extends { id: string }> = {
  initialItems: T[];
  initialCursor: string | null;
  fetchMore: (cursor: string) => Promise<Page<T>>;
  renderItem: (item: T, index: number) => React.ReactNode;
};

export function InfiniteFeed<T extends { id: string }>(props: Props<T>) {
  const [items, setItems] = useState(props.initialItems);
  const [cursor, setCursor] = useState(props.initialCursor);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef(false);
  const fetchMoreRef = useRef(props.fetchMore);
  useEffect(() => { fetchMoreRef.current = props.fetchMore; });

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !cursor) return;
    const obs = new IntersectionObserver(async ([entry]) => {
      if (!entry.isIntersecting || loadingRef.current) return;
      loadingRef.current = true;
      try {
        const page = await fetchMoreRef.current(cursor);
        setItems((prev) => {
          const seen = new Set(prev.map((p) => p.id));
          return [...prev, ...page.items.filter((i) => !seen.has(i.id))];
        });
        setCursor(page.next_cursor);
      } finally {
        loadingRef.current = false;
      }
    }, { rootMargin: "400px 0px" });
    obs.observe(el);
    return () => obs.disconnect();
  }, [cursor]);

  return (
    <>
      {items.map((item, index) => (
        <React.Fragment key={item.id}>
          {props.renderItem(item, index)}
        </React.Fragment>
      ))}
      {cursor && <div ref={sentinelRef} aria-hidden className="feed-sentinel" />}
    </>
  );
}
