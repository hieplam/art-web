// web/app/upload/page.tsx
import { redirect } from "next/navigation";
import { api, apiBase, forwardCookie, hasAuthCookie, isUnauthenticatedError, publicApiBase } from "@/lib/api";
import { ArtworkUploader } from "@/components/ArtworkUploader";
import type { User } from "@/lib/types";

export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

export default async function UploadPage() {
  const serverBase = apiBase();
  const browserBase = publicApiBase();
  if (!hasAuthCookie()) redirect(`${browserBase}/auth/google/start`);

  try {
    await api<User>({ base: serverBase, path: "/me", cookie: forwardCookie() });
  } catch (e) {
    if (isUnauthenticatedError(e)) {
      redirect(`${browserBase}/auth/google/start`);
    }
    throw e;
  }

  return (
    <main style={{ maxWidth: "640px", margin: "0 auto", padding: "32px 16px" }}>
      <header style={{ marginBottom: "20px" }}>
        <div className="detail-eyebrow">New artwork</div>
        <h1 style={{ fontSize: "1.5rem", marginTop: "4px" }}>Add work to your portfolio</h1>
      </header>
      <div className="panel">
        <ArtworkUploader apiBase={browserBase} />
      </div>
    </main>
  );
}
