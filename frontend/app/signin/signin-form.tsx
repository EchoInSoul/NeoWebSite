"use client";

import { useId, useRef, useState, type FormEvent } from "react";
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
import { useSigninForm } from "@/hooks/use-signin";

export function SigninForm() {
  const t = useTranslations("auth.signin");
  const tCommon = useTranslations("common");
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
  } = useSigninForm();

  const formRef = useRef<HTMLFormElement>(null);
  const identifierFieldId = useId();
  const passwordFieldId = useId();
  const codeFieldId = useId();

  const [isPasswordVisible, setIsPasswordVisible] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const field = step === "verify" ? await submitCode() : await submit();
    if (field) {
      formRef.current
        ?.querySelector<HTMLInputElement>(`[name="${field}"]`)
        ?.focus();
    }
  }

  return (
    <form
      ref={formRef}
      className="flex flex-col gap-5"
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
            <Label htmlFor={passwordFieldId}>{t("password")}</Label>
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
    </form>
  );
}
