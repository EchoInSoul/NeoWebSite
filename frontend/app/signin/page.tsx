import type { Metadata } from "next";
import Link from "next/link";
import { getTranslations } from "next-intl/server";

import { AuthFrame } from "@/components/auth/auth-frame";
import { SigninForm } from "./signin-form";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("auth.signin");

  return {
    title: t("title"),
  };
}

export default async function SigninPage() {
  const t = await getTranslations("auth.signin");

  return (
    <AuthFrame
      footer={
        <p className="mt-5 flex items-center justify-center gap-1 text-center text-sm text-muted-foreground">
          {t("prompt")}
          <Link
            href="/signup"
            className="font-medium text-primary underline-offset-4 hover:underline"
          >
            {t("link")}
          </Link>
        </p>
      }
    >
      <div>
        <h1 className="text-xl font-semibold">{t("title")}</h1>
        <p className="mt-1.5 text-sm text-muted-foreground">{t("subtitle")}</p>
      </div>
      <div className="mt-6">
        <SigninForm />
      </div>
    </AuthFrame>
  );
}
