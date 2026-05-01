// web/app/art/[id]/not-found.tsx
// Critical: this page MUST NOT echo any artwork metadata.
// Spec §8.6.1 case 9 — anonymous request to a private artwork id renders this page,
// and the HTML body must not contain the artwork's title, description, or id.
export default function NotFound() {
  return <main className="p-8 text-center"><h1>Not found</h1></main>;
}
