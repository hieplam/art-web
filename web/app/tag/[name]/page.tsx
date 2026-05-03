// web/app/tag/[name]/page.tsx
import { api, apiBase } from "@/lib/api";
import { Masonry } from "@/components/Masonry";
import { ArtCard } from "@/components/ArtCard";
import type { Feed } from "@/lib/types";

// Tag pages must reflect visibility changes immediately; caching here leaks
// stale public HTML after an artwork is made private.
export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

export default async function Tag({ params }: { params: { name: string } }) {
  const name = decodeURIComponent(params.name);
  const feed = await api<Feed>({
    base: apiBase(),
    path: `/tags/${encodeURIComponent(params.name)}?limit=24`,
  });
  return (
    <main>
      <header className="page-header">
        <h1 style={{ fontFamily: "var(--font-mono)", fontWeight: 500 }}>#{name}</h1>
        <div className="slug">{feed.items.length} works tagged</div>
      </header>
      {feed.items.length === 0 ? (
        <div className="empty">nothing tagged #{name} yet</div>
      ) : (
        <Masonry>{feed.items.map((it) => <ArtCard key={it.id} item={it} />)}</Masonry>
      )}
    </main>
  );
}
