import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";

import {
  getPublicSettings,
  isPasswordResetEnabled,
} from "@/lib/public-settings";
import { ResetRequestForm } from "./reset-request-form";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.reset");

  return {
    title: t("title"),
    robots: { index: false, follow: false },
  };
}

export default async function ResetPasswordPage() {
  const settings = await getPublicSettings();

  if (!isPasswordResetEnabled(settings)) {
    notFound();
  }

  return <ResetRequestForm />;
}
