// web/components/ArtCard.tsx
import Image from "next/image";
import Link from "next/link";
import type { ArtworkSummary } from "@/lib/types";
import { blurhashToDataURL } from "@/lib/blurhash";

type Tier = "narrow" | "square" | "wide" | "panoramic";

function tierFor(width: number, height: number): Tier {
  const r = width / height;
  if (r <= 0.8) return "narrow";
  if (r <= 1.2) return "square";
  if (r <= 1.7) return "wide";
  return "panoramic";
}

const SIZES_BY_TIER: Record<Tier, string> = {
  narrow:    "(max-width: 640px) 50vw, (max-width: 1024px) 33vw, (max-width: 1440px) 25vw, 20vw",
  square:    "(max-width: 640px) 50vw, (max-width: 1024px) 33vw, (max-width: 1440px) 25vw, 20vw",
  wide:      "(max-width: 640px) 100vw, (max-width: 1024px) 66vw, (max-width: 1440px) 50vw, 40vw",
  panoramic: "(max-width: 640px) 100vw, (max-width: 1024px) 100vw, (max-width: 1440px) 75vw, 60vw",
};

const SIZES_SPOTLIGHT =
  "(max-width: 640px) 50vw, (max-width: 1024px) 66vw, (max-width: 1440px) 50vw, 40vw";

export function ArtCard({ item, spotlight = false }: { item: ArtworkSummary; spotlight?: boolean }) {
  const c = item.cover;
  const tier = tierFor(c.width, c.height);
  const sizes = spotlight ? SIZES_SPOTLIGHT : SIZES_BY_TIER[tier];
  return (
    <Link
      href={`/art/${item.id}`}
      data-artwork-id={item.id}
      data-tier={tier}
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
