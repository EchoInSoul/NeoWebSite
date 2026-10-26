"use client";

import Link from "next/link";
import { useId, useState, type FormEvent } from "react";
import { Eye, EyeOff, LoaderCircle } from "lucide-react";
import { useTranslations } from "next-intl";

import { AuthFrame } from "@/components/auth/auth-frame";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";
import { usePasswordResetConfirm } from "@/hooks/use-password-reset-confirm";
import { PASSWORD_MIN_LENGTH } from "@/lib/password";

export function ResetConfirmForm({ token }: { token: string }) {
  const t = useTranslations("auth.reset");
  const tCommon = useTranslations("common");
  const {
    values,
    isPending,
    serviceDown,
    retryRemaining,
    retryLabel,
    invalidToken,
    changeField,
    submit,
  } = usePasswordResetConfirm(token);

  const passwordFieldId = useId();
  const confirmFieldId = useId();
  const [isPasswordVisible, setIsPasswordVisible] = useState(false);
  const [isConfirmVisible, setIsConfirmVisible] = useState(false);

  const submitLabel =
    retryLabel ??
    (serviceDown
      ? tCommon("serviceUnavailableButton")
      : isPending
        ? t("submitting")
        : t("submitConfirm"));

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
      <h1 className="text-xl font-semibold">{t("confirmTitle")}</h1>
      <div className="mt-6">
        {invalidToken ? (
          <p className="text-sm text-muted-foreground">
            {t("invalidToken")}{" "}
            <Link
              href="/reset-password"
              className="font-medium text-primary underline-offset-4 hover:underline"
            >
              {t("goRequest")}
            </Link>
          </p>
        ) : (
          <form
            className="flex flex-col gap-5"
            method="post"
            onSubmit={handleSubmit}
            noValidate
          >
            <div className="flex flex-col gap-2">
              <Label htmlFor={passwordFieldId}>{t("password")}</Label>
              <InputGroup>
                <InputGroupInput
                  id={passwordFieldId}
                  name="password"
                  type={isPasswordVisible ? "text" : "password"}
                  autoComplete="new-password"
                  required
                  value={values.password}
                  onChange={(event) =>
                    changeField("password", event.target.value)
                  }
                />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton
                    size="icon-xs"
                    aria-label={
                      isPasswordVisible
                        ? tCommon("hidePassword")
                        : tCommon("showPassword")
                    }
                    onClick={() => setIsPasswordVisible((visible) => !visible)}
                  >
                    {isPasswordVisible ? <EyeOff /> : <Eye />}
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
              <p className="text-xs text-muted-foreground">
                {t("passwordHint", { minLength: PASSWORD_MIN_LENGTH })}
              </p>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor={confirmFieldId}>{t("confirmPassword")}</Label>
              <InputGroup>
                <InputGroupInput
                  id={confirmFieldId}
                  name="confirmPassword"
                  type={isConfirmVisible ? "text" : "password"}
                  autoComplete="new-password"
                  required
                  value={values.confirmPassword}
                  onChange={(event) =>
                    changeField("confirmPassword", event.target.value)
                  }
                />
                <InputGroupAddon align="inline-end">
                  <InputGroupButton
                    size="icon-xs"
                    aria-label={
                      isConfirmVisible
                        ? tCommon("hidePassword")
                        : tCommon("showPassword")
                    }
                    onClick={() => setIsConfirmVisible((visible) => !visible)}
                  >
                    {isConfirmVisible ? <EyeOff /> : <Eye />}
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
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
        )}
      </div>
    </AuthFrame>
  );
}
