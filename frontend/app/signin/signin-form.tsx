"use client";

import { useId, useState, type FormEvent } from "react";
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

export function SigninForm() {
  const t = useTranslations("auth.signin");
  const tCommon = useTranslations("common");
  const identifierFieldId = useId();
  const passwordFieldId = useId();
  const [isPasswordVisible, setIsPasswordVisible] = useState(false);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={handleSubmit} noValidate>
      <fieldset className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor={identifierFieldId}>{t("identifier")}</Label>
          <Input
            id={identifierFieldId}
            name="identifier"
            type="text"
            autoComplete="username"
            required
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

      <Button type="submit" className="w-full">
        {t("submit")}
      </Button>
    </form>
  );
}
