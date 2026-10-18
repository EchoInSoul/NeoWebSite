import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { LoginControlForm } from "./login-control-form";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("settings");

  return {
    title: t("loginControl"),
  };
}

export default async function SettingsLoginPage() {
  const t = await getTranslations("settings");

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">
        {t("loginControl")}
      </h1>
      <LoginControlForm />
    </div>
  );
}
