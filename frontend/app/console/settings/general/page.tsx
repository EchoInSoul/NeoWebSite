import { getTranslations } from "next-intl/server";

export default async function SettingsGeneralPage() {
  const t = await getTranslations("settings");

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("general")}</h1>
    </div>
  );
}
