// web/components/ArtworkUploader.tsx
"use client";
import { useState } from "react";

function uuidv4(): string {
  return crypto.randomUUID();
}

export function ArtworkUploader({ apiBase }: { apiBase: string }) {
  const [title, setTitle] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setSubmitting(true);
    setError(null);
    try {
      const create = await fetch(`${apiBase}/artworks`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title, visibility: "private" }),
      });
      if (!create.ok) throw new Error("create failed");
      const art = await create.json();

      const manifest = files.map((f, i) => ({
        client_image_id: uuidv4(),
        position: i,
        content_type: f.type,
      }));
      const fd = new FormData();
      fd.set("manifest", JSON.stringify(manifest));
      for (const f of files) fd.append("files", f);

      const upload = await fetch(`${apiBase}/artworks/${art.id}/images`, {
        method: "POST", credentials: "include", body: fd,
      });
      if (!upload.ok) throw new Error("upload failed");

      window.location.href = `/art/${art.id}`;
    } catch (e) {
      setError(e instanceof Error ? e.message : "unknown");
    } finally { setSubmitting(false); }
  }

  return (
    <div className="space-y-3">
      <input className="border p-2 w-full" placeholder="Title"
             value={title} onChange={(e) => setTitle(e.target.value)} />
      <input type="file" multiple accept="image/jpeg,image/png"
             onChange={(e) => setFiles(Array.from(e.target.files ?? []))} />
      <button className="bg-black text-white px-4 py-2"
              disabled={!title || files.length === 0 || submitting}
              onClick={submit}>
        {submitting ? "Uploading…" : "Upload"}
      </button>
      {error && <div className="text-red-600">{error}</div>}
    </div>
  );
}
