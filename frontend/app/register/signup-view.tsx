"use client";

import Link from "next/link";
import { useTranslations } from "next-intl";

import { AuthFrame } from "@/components/auth/auth-frame";
import type { CaptchaConfig } from "@/lib/public-settings";
import { SignupForm } from "./signup-form";

export function SignupView({ captcha }: { captcha: CaptchaConfig | null }) {
  const t = useTranslations("auth.signup");

  return (
    <AuthFrame
      footer={
        <p className="mt-5 flex items-center justify-center gap-1 text-center text-sm text-muted-foreground">
          {t("prompt")}
          <Link
            href="/login"
            className="font-medium text-primary underline-offset-4 hover:underline"
          >
            {t("link")}
          </Link>
        </p>
      }
    >
      <h1 className="text-xl font-semibold">{t("title")}</h1>
      <div className="mt-6">
        <SignupForm captcha={captcha} />
      </div>
    </AuthFrame>
  );
}
