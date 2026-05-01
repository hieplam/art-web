// web/app/upload/page.tsx
import { redirect } from "next/navigation";
import { api, apiBase, ApiClientError, forwardCookie, hasAuthCookie, publicApiBase } from "@/lib/api";
import { ArtworkUploader } from "@/components/ArtworkUploader";
import type { User } from "@/lib/types";

export const dynamic = "force-dynamic";

export default async function UploadPage() {
  const serverBase = apiBase();
  const browserBase = publicApiBase();
  if (!hasAuthCookie()) redirect(`${browserBase}/auth/google/start`);

  try {
    await api<User>({ base: serverBase, path: "/me", cookie: forwardCookie() });
  } catch (e) {
    if (e instanceof ApiClientError && e.status === 401) {
      redirect(`${browserBase}/auth/google/start`);
    }
    throw e;
  }

  return (
    <main className="max-w-2xl mx-auto p-4">
      <h1 className="text-2xl mb-3">New artwork</h1>
      <ArtworkUploader apiBase={browserBase} />
    </main>
  );
}
