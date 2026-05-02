// web/components/SettingsClient.tsx
"use client";
import type { User } from "@/lib/types";

export function SettingsClient({ me, apiBase }: { me: User; apiBase: string }) {
  return (
    <main style={{ maxWidth: "640px", margin: "0 auto", padding: "32px 16px" }}>
      <header style={{ marginBottom: "20px" }}>
        <div className="detail-eyebrow">Account</div>
        <h1 style={{ fontSize: "1.5rem", marginTop: "4px" }}>Settings</h1>
      </header>
      <div className="panel space-y-3">
        <dl className="detail-meta-grid">
          <dt>Display name</dt>
          <dd style={{ fontFamily: "var(--font-sans)", fontSize: "0.9375rem" }}>{me.display_name}</dd>
          <dt>Slug</dt>
          <dd>@{me.slug}</dd>
        </dl>
        <form method="POST" action={`${apiBase}/auth/logout`} style={{ marginTop: "16px" }}>
          <button type="submit" className="btn btn-ghost">Sign out</button>
        </form>
      </div>
    </main>
  );
}
