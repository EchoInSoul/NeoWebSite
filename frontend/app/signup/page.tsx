import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { getAuthenticatedUser } from "@/lib/auth-session";
import {
  getCaptchaConfig,
  getPublicSettings,
  isSignupOpen,
} from "@/lib/public-settings";
import { SignupView } from "./signup-view";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signup");

  return {
    title: t("title"),
  };
}

export default async function SignupPage() {
  const settings = await getPublicSettings();

  if (!isSignupOpen(settings)) {
    notFound();
  }

  const captcha = getCaptchaConfig(settings);

  let user;
  try {
    user = await getAuthenticatedUser();
  } catch {
    return <SignupView captcha={captcha} />;
  }

  if (user) {
    redirect("/console");
  }

  return <SignupView captcha={captcha} />;
}
