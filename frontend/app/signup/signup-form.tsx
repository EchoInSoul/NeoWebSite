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
import { PASSWORD_MIN_LENGTH, useSignupForm } from "@/hooks/use-signup";

export function SignupForm() {
  const t = useTranslations("auth.signup");
  const tCommon = useTranslations("common");
  const { values, errors, confirmError, isPending, changeField, submit } =
    useSignupForm();

  const formRef = useRef<HTMLFormElement>(null);
  const emailFieldId = useId();
  const nameFieldId = useId();
  const passwordFieldId = useId();
  const confirmFieldId = useId();
  const emailErrorId = useId();
  const nameErrorId = useId();
  const passwordErrorId = useId();
  const confirmErrorId = useId();
  const formErrorId = useId();

  const [isPasswordVisible, setIsPasswordVisible] = useState(false);
  const [isConfirmVisible, setIsConfirmVisible] = useState(false);

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
          <Label htmlFor={emailFieldId}>{t("email")}</Label>
          <Input
            id={emailFieldId}
            name="email"
            type="email"
            autoComplete="email"
            required
            aria-invalid={errors.email !== undefined}
            aria-describedby={errors.email ? emailErrorId : undefined}
            value={values.email}
            onChange={(event) => changeField("email", event.target.value)}
          />
          <FieldError id={emailErrorId} message={errors.email} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={nameFieldId}>{t("name")}</Label>
          <Input
            id={nameFieldId}
            name="name"
            type="text"
            autoComplete="username"
            required
            aria-invalid={errors.name !== undefined}
            aria-describedby={errors.name ? nameErrorId : undefined}
            value={values.name}
            onChange={(event) => changeField("name", event.target.value)}
          />
          <FieldError id={nameErrorId} message={errors.name} />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={passwordFieldId}>{t("password")}</Label>
          <InputGroup>
            <InputGroupInput
              id={passwordFieldId}
              name="password"
              type={isPasswordVisible ? "text" : "password"}
              autoComplete="new-password"
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
          <p className="text-xs text-muted-foreground">
            {t("passwordHint", { minLength: PASSWORD_MIN_LENGTH })}
          </p>
          <FieldError id={passwordErrorId} message={errors.password} />
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
              aria-invalid={confirmError !== undefined}
              aria-describedby={confirmError ? confirmErrorId : undefined}
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
          <FieldError id={confirmErrorId} message={confirmError} />
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
