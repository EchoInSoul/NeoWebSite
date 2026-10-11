import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("console");

  return {
    title: t("title"),
  };
}

export default function ConsolePage() {
  return null;
}
