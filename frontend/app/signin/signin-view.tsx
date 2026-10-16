"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";

import { AuthFrame } from "@/components/auth/auth-frame";
import { useSignupNotice } from "@/hooks/use-signup-notice";
import { SigninForm } from "./signin-form";

export function SigninView({ signupOpen }: { signupOpen: boolean }) {
  const t = useTranslations("auth.signin");

  useSignupNotice();

  return (
    <AuthFrame
      footer={
        signupOpen ? (
          <p className="mt-5 flex items-center justify-center gap-1 text-center text-sm text-muted-foreground">
            {t("prompt")}
            <Link
              href="/signup"
              className="font-medium text-primary underline-offset-4 hover:underline"
            >
              {t("link")}
            </Link>
          </p>
        ) : undefined
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
