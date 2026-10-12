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
  const { values, isPending, changeField, submit } = useSigninForm();

  const formRef = useRef<HTMLFormElement>(null);
  const identifierFieldId = useId();
  const passwordFieldId = useId();

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
            value={values.identifier}
            onChange={(event) => changeField("identifier", event.target.value)}
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
        </div>
      </fieldset>

      <Button type="submit" className="w-full" disabled={isPending}>
        {t("submit")}
      </Button>
    </form>
  );
}
