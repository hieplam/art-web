// web/lib/types.ts
// Mirrors contracts §8 byte-for-byte. Any change here MUST also land in
// the Go API (artwork response renderer) and the contracts doc.

export type User = {
  id: string;
  display_name: string;
  slug: string;
  avatar_url: string | null;
};

export type ImageRef = {
  id: string;
  url: string;
  width: number;
  height: number;
  blurhash: string;
  position: number;
};

export type ArtworkSummary = {
  id: string;
  title: string;
  visibility: "public" | "private";
  published_at: string | null;
  created_at: string;
  cover: ImageRef;
  artist: User;
};

export type ArtworkDetail = ArtworkSummary & {
  description: string | null;
  tags: string[];
  images: ImageRef[];
};

export type Feed = {
  items: ArtworkSummary[];
  next_cursor: string | null;
};

export type UserProfile = {
  user: User;
  artworks: ArtworkSummary[];
  next_cursor: string | null;
};

export type ApiError = {
  error: string;
  message: string;
};
