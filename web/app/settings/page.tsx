// web/app/settings/page.tsx
import { redirect } from "next/navigation";
import { api, apiBase, forwardCookie, hasAuthCookie, isUnauthenticatedError, publicApiBase } from "@/lib/api";
import { SettingsClient } from "@/components/SettingsClient";
import type { User } from "@/lib/types";

export const dynamic = "force-dynamic";
export const fetchCache = "force-no-store";

export default async function Settings() {
  const serverBase = apiBase();
  const browserBase = publicApiBase();
  if (!hasAuthCookie()) redirect(`${browserBase}/auth/google/start`);
  let me: User;
  try {
    me = await api<User>({ base: serverBase, path: "/me", cookie: forwardCookie() });
  } catch (e) {
    if (isUnauthenticatedError(e)) {
      redirect(`${browserBase}/auth/google/start`);
    }
    throw e;
  }
  return <SettingsClient me={me} apiBase={browserBase} />;
}
