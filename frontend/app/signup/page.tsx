import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { getAuthenticatedUser } from "@/lib/auth-session";
import { SignupView } from "./signup-view";

export const instant = false;

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signup");

  return {
    title: t("title"),
  };
}

export default async function SignupPage() {
  let user;
  try {
    user = await getAuthenticatedUser();
  } catch {
    return <SignupView />;
  }

  if (user) {
    redirect("/console");
  }

  return <SignupView />;
}
