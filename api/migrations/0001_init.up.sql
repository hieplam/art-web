-- api/migrations/0001_init.up.sql
CREATE TABLE users (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  oauth_provider  text NOT NULL,
  oauth_subject   text NOT NULL,
  email           text NOT NULL,
  display_name    text NOT NULL,
  slug            text UNIQUE NOT NULL,
  avatar_url      text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (oauth_provider, oauth_subject)
);

CREATE TABLE artworks (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title          text NOT NULL,
  description    text,
  visibility     text NOT NULL CHECK (visibility IN ('public','private')),
  cover_position int NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now(),
  published_at   timestamptz
);

CREATE INDEX artworks_public_feed_idx ON artworks (published_at DESC)
  WHERE visibility = 'public' AND published_at IS NOT NULL;

CREATE INDEX artworks_by_user_idx ON artworks (user_id, created_at DESC);

CREATE TABLE artwork_images (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  artwork_id      uuid NOT NULL REFERENCES artworks(id) ON DELETE CASCADE,
  client_image_id text NOT NULL CHECK (client_image_id <> ''),
  storage_key     text NOT NULL,
  source_sha256   text NOT NULL,
  width           int NOT NULL,
  height          int NOT NULL,
  byte_size       int NOT NULL,
  content_type    text NOT NULL,
  position        int NOT NULL,
  blurhash        text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (artwork_id, client_image_id),
  UNIQUE (artwork_id, position)
);

CREATE TABLE tags (
  id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text UNIQUE NOT NULL
);

CREATE TABLE artwork_tags (
  artwork_id uuid NOT NULL REFERENCES artworks(id) ON DELETE CASCADE,
  tag_id     uuid NOT NULL REFERENCES tags(id),
  PRIMARY KEY (artwork_id, tag_id)
);
CREATE INDEX artwork_tags_by_tag_idx ON artwork_tags (tag_id);
