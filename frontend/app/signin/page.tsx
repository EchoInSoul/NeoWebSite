import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { getAuthenticatedUser } from "@/lib/auth-session";
import { getPublicSettings, isSignupOpen } from "@/lib/public-settings";
import { SigninView } from "./signin-view";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signin");

  return {
    title: t("title"),
  };
}

export default async function SigninPage() {
  const signupOpen = isSignupOpen(await getPublicSettings());

  let user;
  try {
    user = await getAuthenticatedUser();
  } catch {
    return <SigninView signupOpen={signupOpen} />;
  }

  if (user) {
    redirect("/console");
  }

  return <SigninView signupOpen={signupOpen} />;
}
