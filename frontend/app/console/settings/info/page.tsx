import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { getSiteSettings } from "@/lib/admin-settings";

import { SiteSettingsForm } from "./site-settings-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");

  return {
    title: t("siteSettings"),
  };
}

export default async function SettingsInfoPage() {
  const t = await getTranslations("settings");
  const read = await getSiteSettings();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("general")}</h1>
      {read.status === "ok" ? (
        <SiteSettingsForm settings={read.values} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {read.status === "forbidden" ? t("noPermission") : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
