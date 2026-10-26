"use client";

import Link from "next/link";
import { useId, type FormEvent } from "react";
import { LoaderCircle } from "lucide-react";
import { useTranslations } from "next-intl";

import { AuthFrame } from "@/components/auth/auth-frame";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePasswordResetRequest } from "@/hooks/use-password-reset-request";

export function ResetRequestForm() {
  const t = useTranslations("auth.reset");
  const tCommon = useTranslations("common");
  const {
    email,
    isPending,
    serviceDown,
    retryRemaining,
    retryLabel,
    changeEmail,
    submit,
  } = usePasswordResetRequest();
  const emailFieldId = useId();

  const submitLabel =
    retryLabel ??
    (serviceDown
      ? tCommon("serviceUnavailableButton")
      : isPending
        ? t("submitting")
        : t("submit"));

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await submit();
  }

  return (
    <AuthFrame
      footer={
        <p className="mt-5 flex items-center justify-center text-center text-sm text-muted-foreground">
          <Link
            href="/login"
            className="font-medium text-primary underline-offset-4 hover:underline"
          >
            {t("backToLogin")}
          </Link>
        </p>
      }
    >
      <h1 className="text-xl font-semibold">{t("title")}</h1>
      <div className="mt-6">
        <form
          className="flex flex-col gap-5"
          method="post"
          onSubmit={handleSubmit}
          noValidate
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor={emailFieldId}>{t("email")}</Label>
            <Input
              id={emailFieldId}
              name="email"
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(event) => changeEmail(event.target.value)}
            />
          </div>

          <Button
            type="submit"
            className="w-full"
            disabled={isPending || retryRemaining > 0}
            aria-busy={isPending}
          >
            {isPending ? (
              <>
                <LoaderCircle className="animate-spin" aria-hidden="true" />
                {t("submitting")}
              </>
            ) : (
              submitLabel
            )}
          </Button>
        </form>
      </div>
    </AuthFrame>
  );
}
