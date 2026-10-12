import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { SigninView } from "./signin-view";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signin");

  return {
    title: t("title"),
  };
}

export default function SigninPage() {
  return <SigninView />;
}
