import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { getMailSettings } from "@/lib/admin-settings";
import { getAuthenticatedUser } from "@/lib/auth-session";

import { MailSettingsForm } from "./mail-settings-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");

  return {
    title: t("mail"),
  };
}

export default async function SettingsMailPage() {
  const t = await getTranslations("settings");
  const [read, user] = await Promise.all([
    getMailSettings(),
    getAuthenticatedUser().catch(() => null),
  ]);
  const canSendTestEmail =
    user?.group.systemKey === "owner" || user?.group.systemKey === "admin";

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("mail")}</h1>
      {read.status === "ok" ? (
        <MailSettingsForm
          settings={read.values}
          currentUserEmail={user?.email ?? ""}
          canSendTestEmail={canSendTestEmail}
        />
      ) : (
        <p className="text-sm text-muted-foreground">
          {read.status === "forbidden" ? t("noPermission") : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
