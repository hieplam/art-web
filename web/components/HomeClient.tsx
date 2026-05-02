// web/components/HomeClient.tsx
"use client";
import { InfiniteFeed } from "./InfiniteFeed";
import { Masonry } from "./Masonry";
import { ArtCard } from "./ArtCard";
import type { ArtworkSummary, Feed } from "@/lib/types";
import { appendCursor } from "@/lib/cursor";

export function HomeClient(props: {
  initialItems: ArtworkSummary[];
  initialCursor: string | null;
  apiBase: string;
}) {
  return (
    <Masonry>
      <InfiniteFeed<ArtworkSummary>
        initialItems={props.initialItems}
        initialCursor={props.initialCursor}
        fetchMore={async (c) => {
          const r = await fetch(`${props.apiBase}${appendCursor("/artworks?limit=24", c)}`);
          if (!r.ok) throw new Error(`feed fetch failed: ${r.status}`);
          return r.json() as Promise<Feed>;
        }}
        renderItem={(it) => <ArtCard key={it.id} item={it} />}
      />
    </Masonry>
  );
}
