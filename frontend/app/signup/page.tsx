import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { SignupView } from "./signup-view";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signup");

  return {
    title: t("title"),
  };
}

export default function SignupPage() {
  return <SignupView />;
}
