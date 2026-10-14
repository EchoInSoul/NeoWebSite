"use client";

import { useState, type KeyboardEvent } from "react";
import { Eye, EyeOff, ShieldOff } from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { useTwoFactorDisable } from "@/hooks/use-two-factor-disable";

export function TwoFactorDisablePanel({ onDone }: { onDone: () => void }) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const { password, isSubmitting, changePassword, submit } =
    useTwoFactorDisable(onDone);
  const [isPasswordVisible, setIsPasswordVisible] = useState(false);

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      void submit();
    }
  }

  return (
    <ConsoleSettingsRow
      control={
        <div className="flex w-70 items-center gap-2">
          <InputGroup className="min-w-0 flex-1">
            <InputGroupInput
              aria-label={t("currentPassword")}
              autoComplete="current-password"
              disabled={isSubmitting}
              onChange={(event) => changePassword(event.target.value)}
              onKeyDown={handleKeyDown}
              type={isPasswordVisible ? "text" : "password"}
              value={password}
            />
            <InputGroupAddon align="inline-end">
              <InputGroupButton
                size="icon-xs"
                aria-label={
                  isPasswordVisible
                    ? tCommon("hidePassword")
                    : tCommon("showPassword")
                }
                disabled={isSubmitting}
                onClick={() => setIsPasswordVisible((visible) => !visible)}
              >
                {isPasswordVisible ? <EyeOff /> : <Eye />}
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
          <Button
            disabled={password === "" || isSubmitting}
            onClick={() => void submit()}
            type="button"
            variant="destructive"
          >
            {t("twoFactorDisableSubmit")}
          </Button>
        </div>
      }
      icon={<ShieldOff />}
      label={t("twoFactorDisableLabel")}
    />
  );
}
