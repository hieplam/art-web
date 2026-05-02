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
  const images = data.images ?? [];
  const tags = data.tags ?? [];
  const cover = images[0] ?? data.cover;
  const rest = images.slice(1);

  return (
    <article className="detail">
      <div className="detail-stage">
        {cover && (
          <Image
            src={cover.url}
            width={cover.width}
            height={cover.height}
            alt={data.title}
            className="detail-image"
            placeholder="blur"
            blurDataURL={blurhashToDataURL(cover.blurhash)}
            sizes="(max-width: 1023px) 100vw, calc(100vw - 392px)"
            priority
          />
        )}
      </div>

      <aside className="detail-side">
        <div className="detail-eyebrow">
          {data.visibility === "private" ? "Private · Owner" : "Public"}
        </div>
        <h1 className="detail-title">{data.title}</h1>
        <Link href={`/u/${data.artist.slug}`} className="detail-artist">
          {data.artist.display_name}
        </Link>
        {data.published_at && (
          <time className="detail-date">{data.published_at.slice(0, 10)}</time>
        )}

        {data.description && <p className="detail-description">{data.description}</p>}

        {cover && (
          <dl className="detail-meta-grid">
            <dt>Format</dt>
            <dd>PNG</dd>
            <dt>Dimensions</dt>
            <dd>{cover.width} × {cover.height}</dd>
            <dt>Plates</dt>
            <dd>{images.length || 1}</dd>
          </dl>
        )}

        {tags.length > 0 && (
          <div className="detail-tags">
            {tags.map((t) => (
              <Link key={t} href={`/tag/${encodeURIComponent(t)}`} className="tag">
                #{t}
              </Link>
            ))}
          </div>
        )}
      </aside>

      {rest.length > 0 && (
        <div className="detail-stage detail-extra">
          {rest.map((im) => (
            <Image
              key={im.id}
              src={im.url}
              width={im.width}
              height={im.height}
              alt={data.title}
              className="detail-image"
              placeholder="blur"
              blurDataURL={blurhashToDataURL(im.blurhash)}
              sizes="(max-width: 1023px) 100vw, calc(100vw - 392px)"
            />
          ))}
        </div>
      )}
    </article>
  );
}
