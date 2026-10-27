import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("content");

  return {
    title: t("media"),
  };
}

export default async function MediaPage() {
  const t = await getTranslations("content");

  return (
    <div className="mx-auto flex w-full max-w-[1000px] flex-col gap-4 p-4 min-[1200px]:p-6">
      <h1 className="text-xl font-semibold tracking-tight">{t("media")}</h1>
    </div>
  );
}
