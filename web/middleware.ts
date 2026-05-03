// web/middleware.ts
// Why this exists:
// Next.js App Router serializes the request URL (initialCanonicalUrl + parsed
// route segments) into the HTML's RSC payload. Even when /art/[id]/page.tsx
// calls notFound() and our not-found.tsx body is empty, the framework still
// emits the requested id in the document. For a private artwork that isn't
// visible to the requester, that's a metadata leak (privacy spec §8.6.1, case
// 9 + case 25 in the e2e suite).
//
// Solution: probe the API in middleware. If the artwork isn't visible to this
// caller (API → 404), short-circuit with a plain-text response that contains
// no artwork metadata. Visible artworks fall through to the page.
import { NextResponse, type NextRequest } from "next/server";

const ART_DETAIL = /^\/art\/([^/]+)\/?$/;

function resolveApiBase(): string {
  return (
    process.env.API_BASE_INTERNAL ||
    process.env.NEXT_PUBLIC_API_BASE ||
    "http://localhost:8080"
  );
}

export async function middleware(req: NextRequest) {
  if (req.method !== "GET" && req.method !== "HEAD") return NextResponse.next();

  const m = req.nextUrl.pathname.match(ART_DETAIL);
  if (!m) return NextResponse.next();

  const id = m[1];
  const cookie = req.headers.get("cookie") ?? "";

  let upstream: Response;
  try {
    upstream = await fetch(`${resolveApiBase()}/artworks/${encodeURIComponent(id)}`, {
      headers: cookie ? { Cookie: cookie } : {},
      cache: "no-store",
    });
  } catch {
    return NextResponse.next();
  }

  if (upstream.status === 404) {
    return new NextResponse("Not Found", {
      status: 404,
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
        "Cache-Control": "no-store",
        "X-Robots-Tag": "noindex",
      },
    });
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/art/:path*"],
};
