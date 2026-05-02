// web/components/Nav.tsx
import Link from "next/link";
import { api, apiBase, ApiClientError, forwardCookie, hasAuthCookie, publicApiBase } from "@/lib/api";
import type { User } from "@/lib/types";

export async function Nav() {
  const serverBase = apiBase();
  const browserBase = publicApiBase();
  let me: User | null = null;
  if (hasAuthCookie()) {
    try { me = await api<User>({ base: serverBase, path: "/me", cookie: forwardCookie() }); }
    catch (err) {
      if (!(err instanceof ApiClientError && err.status === 401)) throw err;
    }
  }
  return (
    <nav className="flex items-center justify-between px-4 py-2 border-b">
      <Link href="/" className="font-bold">artweb</Link>
      <div className="flex gap-3">
        <Link href="/">Browse</Link>
        {me ? (
          <>
            <Link href="/upload">Upload</Link>
            <Link href={`/u/${me.slug}`}>{me.display_name}</Link>
          </>
        ) : (
          <a href={`${browserBase}/auth/google/start`}>Sign in</a>
        )}
      </div>
    </nav>
  );
}
