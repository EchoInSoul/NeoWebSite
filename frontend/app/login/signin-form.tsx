"use client";

import { useId, useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { Eye, EyeOff } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";
import { CaptchaDialog } from "@/components/auth/captcha-dialog";
import { useCaptchaGate } from "@/hooks/use-captcha-gate";
import { useSigninForm } from "@/hooks/use-signin";
import type { CaptchaConfig } from "@/lib/public-settings";

export function SigninForm({
  captcha,
  passwordResetEnabled,
}: {
  captcha: CaptchaConfig | null;
  passwordResetEnabled: boolean;
}) {
  const t = useTranslations("auth.signin");
  const tCommon = useTranslations("common");
  const captchaGate = useCaptchaGate(captcha);
  const {
    values,
    step,
    code,
    isPending,
    serviceDown,
    retryRemaining,
    retryLabel,
    changeField,
    changeCode,
    backToCredentials,
    submit,
    submitCode,
  } = useSigninForm(captcha !== null, captchaGate.begin);

  const formRef = useRef<HTMLFormElement>(null);
  const identifierFieldId = useId();
  const passwordFieldId = useId();
  const codeFieldId = useId();

  const [isPasswordVisible, setIsPasswordVisible] = useState(false);

  function focusField(field: string | undefined) {
    if (!field) {
      return;
    }
    formRef.current
      ?.querySelector<HTMLInputElement>(`[name="${field}"]`)
      ?.focus();
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const field = step === "verify" ? await submitCode() : await submit();
    focusField(field);
  }

  async function handleCaptchaSolved(token: string) {
    captchaGate.cancel();
    focusField(await submit(token));
  }

  return (
    <form
      ref={formRef}
      className="flex flex-col gap-5"
      method="post"
      onSubmit={handleSubmit}
      noValidate
    >
      {step === "verify" ? (
        <div className="flex flex-col gap-4">
          <h2 className="text-base font-semibold">{t("twoFactorTitle")}</h2>
          <div className="flex flex-col gap-2">
            <Label htmlFor={codeFieldId}>{t("codeLabel")}</Label>
            <Input
              id={codeFieldId}
              name="code"
              type="text"
              autoComplete="one-time-code"
              required
              autoFocus
              value={code}
              onChange={(event) => changeCode(event.target.value)}
            />
          </div>
        </div>
      ) : (
        <fieldset className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor={identifierFieldId}>{t("identifier")}</Label>
            <Input
              id={identifierFieldId}
              name="identifier"
              type="text"
              autoComplete="username"
              required
              value={values.identifier}
              onChange={(event) =>
                changeField("identifier", event.target.value)
              }
            />
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <Label htmlFor={passwordFieldId}>{t("password")}</Label>
              {passwordResetEnabled ? (
                <Link
                  href="/reset-password"
                  className="text-sm font-medium text-primary underline-offset-4 hover:underline"
                >
                  {t("forgotPassword")}
                </Link>
              ) : null}
            </div>
            <InputGroup>
              <InputGroupInput
                id={passwordFieldId}
                name="password"
                type={isPasswordVisible ? "text" : "password"}
                autoComplete="current-password"
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
          </div>
        </fieldset>
      )}

      <Button
        type="submit"
        className="w-full"
        disabled={isPending || retryRemaining > 0}
      >
        {retryLabel ??
          (serviceDown
            ? tCommon("serviceUnavailableButton")
            : step === "verify"
              ? t("verifySubmit")
              : t("submit"))}
      </Button>

      {step === "verify" ? (
        <Button
          type="button"
          variant="outline"
          className="w-full"
          disabled={isPending}
          onClick={backToCredentials}
        >
          {t("backToCredentials")}
        </Button>
      ) : null}

      {captcha === null ? null : (
        <CaptchaDialog
          open={captchaGate.isOpen}
          config={captcha}
          challenge={captchaGate.challenge}
          onOpenChange={captchaGate.handleOpenChange}
          onSolved={handleCaptchaSolved}
        />
      )}
    </form>
  );
}
