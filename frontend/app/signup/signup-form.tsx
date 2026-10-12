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
import { PASSWORD_MIN_LENGTH, useSignupForm } from "@/hooks/use-signup";

export function SignupForm() {
  const t = useTranslations("auth.signup");
  const tCommon = useTranslations("common");
  const { values, isPending, serviceDown, changeField, submit } =
    useSignupForm();

  const formRef = useRef<HTMLFormElement>(null);
  const emailFieldId = useId();
  const nameFieldId = useId();
  const passwordFieldId = useId();
  const confirmFieldId = useId();

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
            value={values.email}
            onChange={(event) => changeField("email", event.target.value)}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={nameFieldId}>{t("name")}</Label>
          <Input
            id={nameFieldId}
            name="name"
            type="text"
            autoComplete="username"
            required
            value={values.name}
            onChange={(event) => changeField("name", event.target.value)}
          />
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
      </fieldset>

      <Button type="submit" className="w-full" disabled={isPending}>
        {serviceDown ? tCommon("serviceUnavailableButton") : t("submit")}
      </Button>
    </form>
  );
}
