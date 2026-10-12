import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";

import { getAuthenticatedUser } from "@/lib/auth-session";
import { SigninView } from "./signin-view";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signin");

  return {
    title: t("title"),
  };
}

export default async function SigninPage() {
  if (await getAuthenticatedUser()) {
    redirect("/console");
  }

  return <SigninView />;
}
