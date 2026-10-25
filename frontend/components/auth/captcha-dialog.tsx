"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { CaptchaChallenge } from "@/lib/captcha-challenge";
import type { CaptchaConfig } from "@/lib/public-settings";
import { BuiltinCaptcha } from "./builtin-captcha";
import { TurnstileCaptcha } from "./turnstile-captcha";

const CLOSE_DELAY_MS = 1618;

export function CaptchaDialog({
  open,
  config,
  challenge = null,
  onOpenChange,
  onSolved,
}: {
  open: boolean;
  config: CaptchaConfig;
  challenge?: CaptchaChallenge | null;
  onOpenChange: (open: boolean) => void;
  onSolved: (token: string) => void;
}) {
  const t = useTranslations("auth");
  const tCommon = useTranslations("common");
  const closeTimerRef = useRef<number | null>(null);
  const [isUnavailable, setIsUnavailable] = useState(false);

  useEffect(() => {
    if (open || closeTimerRef.current === null) {
      return;
    }
    window.clearTimeout(closeTimerRef.current);
    closeTimerRef.current = null;
  }, [open]);

  useEffect(() => {
    return () => {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current);
      }
    };
  }, []);

  function handleOpenChange(next: boolean) {
    if (!next) {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current);
        closeTimerRef.current = null;
      }
      setIsUnavailable(false);
    }
    onOpenChange(next);
  }

  function handleToken(token: string) {
    if (token === "" || closeTimerRef.current !== null) {
      return;
    }
    setIsUnavailable(false);
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null;
      onSolved(token);
    }, CLOSE_DELAY_MS);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange} disablePointerDismissal>
      <DialogContent blockOpenAutoFocus className="gap-3 p-5 sm:max-w-[340px]">
        <DialogHeader>
          <DialogTitle>{t("captchaDialogTitle")}</DialogTitle>
        </DialogHeader>
        {open ? (
          <div className="grid max-w-full min-w-0 gap-4 py-1 [&>*]:max-w-full [&>*]:min-w-0">
            {isUnavailable ? (
              <div className="flex flex-col gap-3">
                <p className="text-muted-foreground">
                  {t("captchaUnavailable")}
                </p>
                <div className="flex justify-end">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setIsUnavailable(false)}
                  >
                    {tCommon("retry")}
                  </Button>
                </div>
              </div>
            ) : config.provider === "turnstile" ? (
              <TurnstileCaptcha
                siteKey={config.siteKey}
                onToken={handleToken}
                onUnavailable={() => setIsUnavailable(true)}
              />
            ) : (
              <BuiltinCaptcha
                scope={config.scope}
                initialChallenge={challenge}
                onToken={handleToken}
              />
            )}
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
