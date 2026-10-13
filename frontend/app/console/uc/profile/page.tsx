import { getTranslations } from "next-intl/server";

export default async function PersonalInfoPage() {
  const t = await getTranslations("uc");

  return (
    <div className="flex flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">
        {t("personalInfo")}
      </h1>
    </div>
  );
}
