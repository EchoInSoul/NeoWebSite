import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { getCaptchaSettings } from "@/lib/admin-settings";

import { CaptchaSettingsForm } from "./captcha-settings-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");

  return {
    title: t("captchaGroup"),
  };
}

export default async function SettingsCaptchaPage() {
  const t = await getTranslations("settings");
  const read = await getCaptchaSettings();

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">
        {t("captchaGroup")}
      </h1>
      {read.status === "ok" ? (
        <CaptchaSettingsForm settings={read.values} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {read.status === "forbidden" ? t("noPermission") : t("loadFailed")}
        </p>
      )}
    </div>
  );
}
