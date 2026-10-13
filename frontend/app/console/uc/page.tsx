import { getTranslations } from "next-intl/server";

export default async function UserCenterPage() {
  const t = await getTranslations("uc");

  return (
    <div className="flex flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("profile")}</h1>
    </div>
  );
}
