import { getTranslations } from "next-intl/server";

import { getCurrentUserSessions } from "@/lib/user-sessions-server";

import { SessionsPanel } from "./sessions-panel";

export default async function SessionsPage() {
  const t = await getTranslations("sessions");
  const sessions = await getCurrentUserSessions();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("title")}</h1>
      <SessionsPanel initialSessions={sessions} />
    </div>
  );
}
