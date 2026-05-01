// web/app/settings/page.tsx
"use client";
import { useEffect, useState } from "react";

export const dynamic = "force-dynamic";

const API = process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";

export default function Settings() {
  const [me, setMe] = useState<any>(null);
  useEffect(() => { fetch(`${API}/me`, { credentials: "include" }).then(r => r.json()).then(setMe); }, []);
  if (!me) return <main className="p-4">Loading…</main>;
  return (
    <main className="max-w-md mx-auto p-4 space-y-3">
      <h1 className="text-2xl">Settings</h1>
      <div>Display name: <strong>{me.display_name}</strong></div>
      <div>Slug: <strong>{me.slug}</strong></div>
      <form method="POST" action={`${API}/auth/logout`}>
        <button type="submit" className="bg-gray-200 px-4 py-2">Sign out</button>
      </form>
    </main>
  );
}
