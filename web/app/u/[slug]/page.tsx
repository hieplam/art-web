// web/app/u/[slug]/page.tsx
import { notFound } from "next/navigation";
import { api, apiBase, ApiClientError, forwardCookie } from "@/lib/api";
import { Masonry } from "@/components/Masonry";
import { ArtCard } from "@/components/ArtCard";
import type { UserProfile } from "@/lib/types";

// Owner sees drafts → uncacheable (contracts §10).
export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

export default async function Profile({ params }: { params: { slug: string } }) {
  let data: UserProfile;
  try {
    data = await api<UserProfile>({
      base: apiBase(),
      path: `/users/${encodeURIComponent(params.slug)}`,
      cookie: forwardCookie(),
    });
  } catch (e) {
    if (e instanceof ApiClientError && e.status === 404) notFound();
    throw e;
  }
  return (
    <main className="p-4">
      <header className="mb-4">
        <h1 className="text-2xl">{data.user.display_name}</h1>
        <div className="text-gray-500">@{data.user.slug}</div>
      </header>
      <Masonry>
        {data.artworks.map((it) => <ArtCard key={it.id} item={it} />)}
      </Masonry>
    </main>
  );
}
