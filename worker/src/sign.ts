// worker/src/sign.ts
const enc = new TextEncoder();

async function importKey(rawBytes: Uint8Array): Promise<CryptoKey> {
  return crypto.subtle.importKey(
    "raw",
    rawBytes,
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign", "verify"],
  );
}

function bytesToHex(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let out = "";
  for (let i = 0; i < bytes.length; i++) {
    out += bytes[i].toString(16).padStart(2, "0");
  }
  return out;
}

export async function computeSignature(
  keyBytes: Uint8Array,
  canonicalPath: string,
  exp: number,
): Promise<string> {
  const stringToSign = `v1|${canonicalPath}|${exp}`;
  const key = await importKey(keyBytes);
  const sig = await crypto.subtle.sign("HMAC", key, enc.encode(stringToSign));
  return bytesToHex(sig);
}

export async function verifySignature(
  keyBytes: Uint8Array,
  canonicalPath: string,
  exp: number,
  presented: string,
): Promise<boolean> {
  const expected = await computeSignature(keyBytes, canonicalPath, exp);
  return constantTimeEqualHex(expected, presented);
}

// Constant-time hex comparison. WebCrypto has no built-in for hex strings,
// so we walk the bytes ourselves. Both inputs MUST be the same length;
// length mismatch returns false without short-circuiting timing.
export function constantTimeEqualHex(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) {
    diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  }
  return diff === 0;
}
