"use client";

import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { Check, Copy, ExternalLink } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const placeholderSecret = "JBSWY3DPEHPK3PXP";
const displayedSecret = placeholderSecret.replace(/(.{4})/g, "$1 ").trim();
const codeLength = 6;
const copiedFeedbackMs = 1600;

const labelClassName = "text-xs leading-4 font-normal text-muted-foreground";
const valueInputClassName = "min-w-0 flex-1 font-mono text-xs";

export function TwoFactorPanel({ email }: { email: string }) {
  const t = useTranslations("uc");
  const [code, setCode] = useState("");
  const [isCopied, setIsCopied] = useState(false);
  const resetTimer = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (resetTimer.current !== null) {
        window.clearTimeout(resetTimer.current);
      }
    };
  }, []);

  const otpauthUrl = `otpauth://totp/NeoBlog:${email}?algorithm=SHA1&digits=6&issuer=NeoBlog&period=30&secret=${placeholderSecret}`;

  async function copySecret() {
    try {
      await navigator.clipboard.writeText(placeholderSecret);
    } catch {
      return;
    }

    setIsCopied(true);

    if (resetTimer.current !== null) {
      window.clearTimeout(resetTimer.current);
    }
    resetTimer.current = window.setTimeout(
      () => setIsCopied(false),
      copiedFeedbackMs,
    );
  }

  function changeCode(event: ChangeEvent<HTMLInputElement>) {
    setCode(event.target.value.replace(/\D/g, "").slice(0, codeLength));
  }

  return (
    <div className="flex items-center gap-8 px-5 pt-4 pb-[18px]">
      <div className="relative w-48 shrink-0 p-2">
        <QRCodeSVG
          bgColor="#ffffff"
          fgColor="#000000"
          level="M"
          marginSize={2}
          size={176}
          value={otpauthUrl}
        />
        <span className="absolute top-0 left-0 size-4 border-t border-l border-border" />
        <span className="absolute top-0 right-0 size-4 border-t border-r border-border" />
        <span className="absolute bottom-0 left-0 size-4 border-b border-l border-border" />
        <span className="absolute bottom-0 right-0 size-4 border-b border-r border-border" />
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
                value={displayedSecret}
              />
              <Button
                aria-label={t("twoFactorCopySecret")}
                onClick={copySecret}
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
                value={otpauthUrl}
              />
              <Button
                aria-label={t("twoFactorOpenAuthenticator")}
                nativeButton={false}
                render={<a href={otpauthUrl} />}
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
            <Input
              autoComplete="one-time-code"
              className="w-70"
              id="two-factor-code"
              inputMode="numeric"
              maxLength={codeLength}
              onChange={changeCode}
              placeholder={t("twoFactorCodePlaceholder")}
              value={code}
            />
            <p aria-live="polite" className="min-h-4 text-xs leading-4" />
          </div>
        </div>
      </div>
    </div>
  );
}
