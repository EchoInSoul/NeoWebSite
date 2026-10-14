import { cookies } from "next/headers";
import { getTranslations } from "next-intl/server";

import { backendUrl } from "@/lib/backend-url";
import { parseUserSessions, type UserSession } from "@/lib/user-sessions";

import { SessionsPanel } from "./sessions-panel";

async function loadInitialSessions(): Promise<UserSession[] | null> {
  const sessionToken = (await cookies()).get("session")?.value;

  if (sessionToken === undefined) {
    return null;
  }

  try {
    const response = await fetch(`${backendUrl}/api/v1/users/me/sessions`, {
      cache: "no-store",
      headers: { cookie: `session=${sessionToken}` },
    });

    if (!response.ok) {
      return null;
    }

    return parseUserSessions(await response.json());
  } catch {
    return null;
  }
}

export default async function SessionsPage() {
  const t = await getTranslations("sessions");
  const sessions = await loadInitialSessions();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("title")}</h1>
      <SessionsPanel initialSessions={sessions} />
    </div>
  );
}
