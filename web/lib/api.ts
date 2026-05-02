// web/lib/api.ts
export class ApiClientError extends Error {
  constructor(public status: number, public body: unknown) {
    super(`API error ${status}`);
  }
}

export type ApiArgs = {
  base: string;
  path: string;
  method?: string;
  body?: BodyInit;
  cookie?: string;        // forwarded as the request `Cookie` header
  headers?: Record<string, string>;
  cache?: RequestCache;
};

export async function api<T = unknown>(args: ApiArgs): Promise<T> {
  const headers: Record<string, string> = { ...(args.headers ?? {}) };
  if (args.cookie) headers["Cookie"] = args.cookie;
  if (args.body && !headers["Content-Type"] && typeof args.body === "string") {
    headers["Content-Type"] = "application/json";
  }
  const resp = await fetch(`${args.base}${args.path}`, {
    method: args.method ?? "GET",
    body: args.body,
    headers,
    cache: args.cache ?? "no-store",
    credentials: "include",
  });
  const text = await resp.text();
  let parsed: unknown;
  try { parsed = text ? JSON.parse(text) : null; } catch { parsed = text; }
  if (!resp.ok) throw new ApiClientError(resp.status, parsed);
  return parsed as T;
}

// Helper for RSC handlers that need to forward the user's cookie to the API.
//
// We build the header explicitly via getAll() rather than relying on
// `cookies().toString()`. The toString shape is not part of Next's
// documented public API and shifts between minor releases (it has shipped
// `name=val; name2=val2`, `name=val&name2=val2`, and a `[object …]`
// stringification across the 13.x → 14.x line). Building the value we
// know the API expects keeps us insulated from that drift.
import { cookies } from "next/headers";

export function forwardCookie(): string | undefined {
  const all = cookies().getAll();
  if (all.length === 0) return undefined;
  return all.map((c) => `${c.name}=${c.value}`).join("; ");
}

export function hasAuthCookie(): boolean {
  return cookies().get("auth") !== undefined;
}

export function publicApiBase(): string {
  return process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080";
}

export function apiBase(): string {
  return process.env.API_BASE_INTERNAL || publicApiBase();
}
