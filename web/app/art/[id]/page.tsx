// web/app/art/[id]/page.tsx
import { notFound } from "next/navigation";
import Image from "next/image";
import Link from "next/link";
import { api, apiBase, ApiClientError, forwardCookie } from "@/lib/api";
import type { ArtworkDetail } from "@/lib/types";
import { blurhashToDataURL } from "@/lib/blurhash";

export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

export default async function Art({ params }: { params: { id: string } }) {
  let data: ArtworkDetail;
  try {
    data = await api<ArtworkDetail>({ base: apiBase(), path: `/artworks/${encodeURIComponent(params.id)}`, cookie: forwardCookie() });
  } catch (e) {
    if (e instanceof ApiClientError && e.status === 404) notFound();
    throw e;
  }
  return (
    <main className="max-w-3xl mx-auto p-4">
      <h1 className="text-2xl mb-1">{data.title}</h1>
      <Link href={`/u/${data.artist.slug}`} className="text-gray-600">
        {data.artist.display_name}
      </Link>
      {data.published_at && (
        <time className="block text-gray-500 text-sm">{data.published_at.slice(0, 10)}</time>
      )}
      {data.description && <p className="my-3 whitespace-pre-wrap">{data.description}</p>}
      <div className="space-y-3 mt-4">
        {data.images.map((im) => (
          <Image key={im.id}
            src={im.url} width={im.width} height={im.height} alt={data.title}
            placeholder="blur" blurDataURL={blurhashToDataURL(im.blurhash)}
            sizes="(max-width: 800px) 100vw, 800px"
          />
        ))}
      </div>
      <div className="flex flex-wrap gap-2 mt-4">
        {data.tags.map((t) => (
          <Link key={t} href={`/tag/${encodeURIComponent(t)}`} className="text-sm px-2 py-1 bg-gray-100 rounded">
            #{t}
          </Link>
        ))}
      </div>
    </main>
  );
}
