"use client";

import { useId, useRef, useState, type FormEvent } from "react";
import { Eye, EyeOff } from "lucide-react";
import { useTranslations } from "next-intl";

import { FieldError } from "@/components/auth/field-error";
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
  const { values, errors, isPending, changeField, submit } = useSigninForm();

  const formRef = useRef<HTMLFormElement>(null);
  const identifierFieldId = useId();
  const passwordFieldId = useId();
  const identifierErrorId = useId();
  const passwordErrorId = useId();
  const formErrorId = useId();

  const [isPasswordVisible, setIsPasswordVisible] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const field = await submit();
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
      <fieldset className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor={identifierFieldId}>{t("identifier")}</Label>
          <Input
            id={identifierFieldId}
            name="identifier"
            type="text"
            autoComplete="username"
            required
            aria-invalid={errors.identifier !== undefined}
            aria-describedby={errors.identifier ? identifierErrorId : undefined}
            value={values.identifier}
            onChange={(event) => changeField("identifier", event.target.value)}
          />
          <FieldError id={identifierErrorId} message={errors.identifier} />
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
              aria-invalid={errors.password !== undefined}
              aria-describedby={errors.password ? passwordErrorId : undefined}
              value={values.password}
              onChange={(event) => changeField("password", event.target.value)}
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
          <FieldError id={passwordErrorId} message={errors.password} />
        </div>
      </fieldset>

      <FieldError id={formErrorId} message={errors.form} />

      <Button
        type="submit"
        className="w-full"
        disabled={isPending}
        aria-describedby={errors.form ? formErrorId : undefined}
      >
        {t("submit")}
      </Button>
    </form>
  );
}
