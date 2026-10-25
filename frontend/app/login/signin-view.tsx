"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";

import { AuthFrame } from "@/components/auth/auth-frame";
import { useSignoutNotice } from "@/hooks/use-signout-notice";
import { useSignupNotice } from "@/hooks/use-signup-notice";
import type { CaptchaConfig } from "@/lib/public-settings";
import { SigninForm } from "./signin-form";

export function SigninView({
  signupOpen,
  captcha,
}: {
  signupOpen: boolean;
  captcha: CaptchaConfig | null;
}) {
  const t = useTranslations("auth.signin");

  useSignupNotice();
  useSignoutNotice();

  return (
    <AuthFrame
      footer={
        signupOpen ? (
          <p className="mt-5 flex items-center justify-center gap-1 text-center text-sm text-muted-foreground">
            {t("prompt")}
            <Link
              href="/register"
              className="font-medium text-primary underline-offset-4 hover:underline"
            >
              {t("link")}
            </Link>
          </p>
        ) : undefined
      }
    >
      <h1 className="text-xl font-semibold">{t("title")}</h1>
      <div className="mt-6">
        <SigninForm captcha={captcha} />
      </div>
    </AuthFrame>
  );
}
