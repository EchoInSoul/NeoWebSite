import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { VerifyForm } from "./verify-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signup");

  return {
    title: t("verifyTitle"),
    referrer: "no-referrer",
    robots: { index: false, follow: false },
  };
}

export default async function VerifyPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token } = await searchParams;

  return <VerifyForm token={typeof token === "string" ? token : ""} />;
}
