"use client";

import { useState, type ChangeEvent, type KeyboardEvent } from "react";
import { Check, Copy, ExternalLink, Eye, EyeOff, KeyRound } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { useTranslations } from "next-intl";

import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useTwoFactorSetup } from "@/hooks/use-two-factor-setup";

import { RecoveryCodesDialog } from "./recovery-codes-dialog";

const labelClassName = "text-xs leading-4 font-normal text-muted-foreground";
const valueInputClassName = "min-w-0 flex-1 font-mono text-xs text-ellipsis";

function groupSecret(secret: string) {
  return secret.replace(/(.{4})/g, "$1 ").trim();
}

export function TwoFactorPanel({ onDone }: { onDone: () => void }) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const { isCopied, copy } = useCopyToClipboard();
  const [isPasswordVisible, setIsPasswordVisible] = useState(false);
  const {
    setup,
    password,
    isLoading,
    code,
    submitState,
    recoveryCodes,
    isBusy,
    startSetup,
    changePassword,
    submitCode,
    changeCode,
    finishRecovery,
  } = useTwoFactorSetup();

  function handleCodeChange(event: ChangeEvent<HTMLInputElement>) {
    changeCode(event.target.value);
  }

  function handleCodeKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      void submitCode(code);
    }
  }

  function handlePasswordKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      void startSetup();
    }
  }

  function finishSetup() {
    finishRecovery();
    onDone();
  }

  if (setup === null) {
    return (
      <ConsoleSettingsRow
        control={
          <div className="flex w-70 items-center gap-2">
            <InputGroup className="min-w-0 flex-1">
              <InputGroupInput
                aria-label={t("currentPassword")}
                autoComplete="current-password"
                disabled={isLoading}
                onChange={(event) => changePassword(event.target.value)}
                onKeyDown={handlePasswordKeyDown}
                type={isPasswordVisible ? "text" : "password"}
                value={password}
              />
              <InputGroupAddon align="inline-end">
                <InputGroupButton
                  aria-label={
                    isPasswordVisible
                      ? tCommon("hidePassword")
                      : tCommon("showPassword")
                  }
                  disabled={isLoading}
                  onClick={() => setIsPasswordVisible((visible) => !visible)}
                  size="icon-xs"
                >
                  {isPasswordVisible ? <EyeOff /> : <Eye />}
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
            <Button
              disabled={password === "" || isLoading}
              onClick={() => void startSetup()}
              type="button"
            >
              {t("twoFactorContinue")}
            </Button>
          </div>
        }
        icon={<KeyRound />}
        label={t("currentPassword")}
      />
    );
  }

  return (
    <>
      <div className="flex items-center gap-8 px-5 pt-4 pb-[18px]">
        <div className="relative w-48 shrink-0 p-2">
          <QRCodeSVG
            bgColor="#ffffff"
            fgColor="#000000"
            level="M"
            marginSize={2}
            size={176}
            value={setup.otpauthUrl}
          />
          <span className="absolute top-0 left-0 size-4 border-t border-l border-border" />
          <span className="absolute top-0 right-0 size-4 border-t border-r border-border" />
          <span className="absolute bottom-0 left-0 size-4 border-b border-l border-border" />
          <span className="absolute right-0 bottom-0 size-4 border-r border-b border-border" />
        </div>

        <div className="flex min-w-0 flex-1 justify-end">
          <div className="flex w-70 flex-col items-start gap-3.5">
            <div className="flex w-full flex-col items-start gap-2">
              <Label className={labelClassName} htmlFor="two-factor-secret">
                {t("twoFactorSecret")}
              </Label>
              <div className="flex w-full items-center gap-2">
                <Input
                  className={valueInputClassName}
                  id="two-factor-secret"
                  readOnly
                  value={groupSecret(setup.secret)}
                />
                <Button
                  aria-label={t("twoFactorCopySecret")}
                  onClick={() => {
                    void copy(setup.secret);
                  }}
                  size="icon"
                  title={t("twoFactorCopySecret")}
                  type="button"
                  variant="outline"
                >
                  {isCopied ? <Check /> : <Copy />}
                </Button>
              </div>
              <span className={labelClassName}>{t("twoFactorOr")}</span>
              <div className="flex w-full items-center gap-2">
                <Input
                  aria-label={t("twoFactorOpenAuthenticator")}
                  className={valueInputClassName}
                  readOnly
                  value={setup.otpauthUrl}
                />
                <Button
                  aria-label={t("twoFactorOpenAuthenticator")}
                  nativeButton={false}
                  render={<a href={setup.otpauthUrl} />}
                  size="icon"
                  title={t("twoFactorOpenAuthenticator")}
                  variant="outline"
                >
                  <ExternalLink />
                </Button>
              </div>
            </div>

            <div className="flex w-full flex-col items-start gap-2">
              <Label className={labelClassName} htmlFor="two-factor-code">
                {t("twoFactorCodePrompt")}
              </Label>
              <div className="flex w-full items-center gap-2">
                <Input
                  aria-invalid={submitState === "invalid"}
                  autoComplete="one-time-code"
                  className="min-w-0 flex-1"
                  disabled={isBusy}
                  id="two-factor-code"
                  inputMode="numeric"
                  maxLength={6}
                  onChange={handleCodeChange}
                  onKeyDown={handleCodeKeyDown}
                  placeholder={t("twoFactorCodePlaceholder")}
                  value={code}
                />
                <Button
                  disabled={code.length !== 6 || isBusy}
                  onClick={() => void submitCode(code)}
                  type="button"
                >
                  {t("save")}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <RecoveryCodesDialog codes={recoveryCodes} onClose={finishSetup} />
    </>
  );
}
