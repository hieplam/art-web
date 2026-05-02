// web/components/SettingsClient.tsx
"use client";
import type { User } from "@/lib/types";

export function SettingsClient({ me, apiBase }: { me: User; apiBase: string }) {
  return (
    <main className="max-w-md mx-auto p-4 space-y-3">
      <h1 className="text-2xl">Settings</h1>
      <div>Display name: <strong>{me.display_name}</strong></div>
      <div>Slug: <strong>{me.slug}</strong></div>
      <form method="POST" action={`${apiBase}/auth/logout`}>
        <button type="submit" className="bg-gray-200 px-4 py-2">Sign out</button>
      </form>
    </main>
  );
}
