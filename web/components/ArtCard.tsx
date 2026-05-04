// web/components/ArtCard.tsx
import Image from "next/image";
import Link from "next/link";
import type { ArtworkSummary } from "@/lib/types";
import { blurhashToDataURL } from "@/lib/blurhash";

export function ArtCard({ item, spotlight = false }: { item: ArtworkSummary; spotlight?: boolean }) {
  const c = item.cover;
  // Spotlight cells are 2x as wide on the grid, so widen the responsive sizes hint accordingly.
  const sizes = spotlight
    ? "(max-width: 640px) 50vw, (max-width: 1024px) 66vw, (max-width: 1440px) 50vw, 40vw"
    : "(max-width: 640px) 50vw, (max-width: 1024px) 33vw, (max-width: 1440px) 25vw, 20vw";
  return (
    <Link
      href={`/art/${item.id}`}
      data-artwork-id={item.id}
      data-spotlight={spotlight ? "true" : undefined}
      className={spotlight ? "art-card art-card-spotlight" : "art-card"}
      aria-label={`${item.title} by ${item.artist.display_name}`}
    >
      <Image
        src={c.url}
        width={c.width}
        height={c.height}
        alt={item.title}
        className="art-card-image"
        placeholder="blur"
        blurDataURL={blurhashToDataURL(c.blurhash)}
        sizes={sizes}
      />
      <div className="art-card-meta">
        <div className="art-card-title">{item.title}</div>
        <div className="art-card-subtitle">{item.artist.display_name}</div>
      </div>
    </Link>
  );
}
