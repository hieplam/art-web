// worker/test/_signer.ts
// In-test ONLY. Never imported from src/. The Worker holds verification
// logic only — never signing — so an attacker who breaches the Worker
// cannot mint new signed URLs.
import { computeSignature } from "../src/sign";

export async function makeSignedURL(
  base: string, // e.g. "https://cdn.example.com"
  keyBytes: Uint8Array,
  storageKey: string, // e.g. "private/abc/img1.jpg"
  expSeconds: number,
  query: Record<string, string> = {},
): Promise<string> {
  const canonical = "/" + storageKey;
  const sig = await computeSignature(keyBytes, canonical, expSeconds);
  const u = new URL(`${base}/img${canonical}`);
  u.searchParams.set("sig", sig);
  u.searchParams.set("exp", String(expSeconds));
  for (const [k, v] of Object.entries(query)) u.searchParams.set(k, v);
  return u.toString();
}
