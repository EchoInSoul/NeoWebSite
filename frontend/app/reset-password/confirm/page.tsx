import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";

import {
  getPublicSettings,
  isPasswordResetEnabled,
} from "@/lib/public-settings";
import { ResetConfirmForm } from "./reset-confirm-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.reset");

  return {
    title: t("confirmTitle"),
    referrer: "no-referrer",
    robots: { index: false, follow: false },
  };
}

export default async function ResetConfirmPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const settings = await getPublicSettings();

  if (!isPasswordResetEnabled(settings)) {
    notFound();
  }

  const { token } = await searchParams;

  return <ResetConfirmForm token={typeof token === "string" ? token : ""} />;
}
