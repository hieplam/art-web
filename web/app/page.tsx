// web/app/page.tsx
import { api, apiBase, publicApiBase } from "@/lib/api";
import { HomeClient } from "@/components/HomeClient";
import type { Feed } from "@/lib/types";

// Without a cache-invalidation bridge from the API into Next, a public page can
// otherwise keep serving artwork that has just been flipped private.
export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

export default async function Home() {
  const initial = await api<Feed>({ base: apiBase(), path: "/artworks?limit=24" });
  return (
    <main>
      <HomeClient initialItems={initial.items} initialCursor={initial.next_cursor} apiBase={publicApiBase()} />
    </main>
  );
}
