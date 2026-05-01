// web/app/page.tsx
import { api, apiBase, publicApiBase } from "@/lib/api";
import { HomeClient } from "@/components/HomeClient";
import type { Feed } from "@/lib/types";

// Public-only: cache freely (contracts §10).
export const revalidate = 60;

export default async function Home() {
  const initial = await api<Feed>({ base: apiBase(), path: "/artworks?limit=24", cache: "force-cache" });
  return (
    <main>
      <HomeClient initialItems={initial.items} initialCursor={initial.next_cursor} apiBase={publicApiBase()} />
    </main>
  );
}
