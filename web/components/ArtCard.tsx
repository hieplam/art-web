// web/components/ArtCard.tsx
import Image from "next/image";
import Link from "next/link";
import type { ArtworkSummary } from "@/lib/types";
import { blurhashToDataURL } from "@/lib/blurhash";

export function ArtCard({ item }: { item: ArtworkSummary }) {
  const c = item.cover;
  return (
    <Link href={`/art/${item.id}`} data-artwork-id={item.id} className="block">
      <Image
        src={c.url}
        width={c.width}
        height={c.height}
        alt={item.title}
        placeholder="blur"
        blurDataURL={blurhashToDataURL(c.blurhash)}
        sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 25vw"
      />
      <div className="px-2 py-1 text-sm">
        <div className="truncate">{item.title}</div>
        <div className="text-gray-500 truncate">{item.artist.display_name}</div>
      </div>
    </Link>
  );
}
