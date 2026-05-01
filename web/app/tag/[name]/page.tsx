// web/app/tag/[name]/page.tsx
import { api, apiBase } from "@/lib/api";
import { Masonry } from "@/components/Masonry";
import { ArtCard } from "@/components/ArtCard";
import type { Feed } from "@/lib/types";

// Public-only by spec §8.6.1 case 5 — cacheable.
export const revalidate = 60;

export default async function Tag({ params }: { params: { name: string } }) {
  const feed = await api<Feed>({
    base: apiBase(),
    path: `/tags/${encodeURIComponent(params.name)}?limit=24`,
    cache: "force-cache",
  });
  return (
    <main className="p-4">
      <h1 className="text-2xl mb-3">#{decodeURIComponent(params.name)}</h1>
      <Masonry>{feed.items.map((it) => <ArtCard key={it.id} item={it} />)}</Masonry>
    </main>
  );
}
