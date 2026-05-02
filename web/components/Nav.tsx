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
    <nav>
      <Link href="/" className="brand">artweb</Link>
      <div className="links">
        <Link href="/">Browse</Link>
        {me ? (
          <>
            <Link href="/upload">Upload</Link>
            <Link href={`/u/${me.slug}`}>{me.display_name}</Link>
          </>
        ) : (
          <a href={`${browserBase}/auth/google/start`} className="signin">Sign in</a>
        )}
      </div>
    </nav>
  );
}
