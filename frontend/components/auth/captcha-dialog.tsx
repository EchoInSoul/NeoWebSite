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
import { TurnstileCaptcha } from "./turnstile-captcha";

const CLOSE_DELAY_MS = 1618;

export function CaptchaDialog({
  open,
  siteKey,
  onOpenChange,
  onSolved,
}: {
  open: boolean;
  siteKey: string;
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
      <DialogContent blockOpenAutoFocus>
        <DialogHeader>
          <DialogTitle>{t("captchaDialogTitle")}</DialogTitle>
        </DialogHeader>
        {open ? (
          isUnavailable ? (
            <div className="flex flex-col gap-3">
              <p className="text-muted-foreground">{t("captchaUnavailable")}</p>
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
          ) : (
            <TurnstileCaptcha
              siteKey={siteKey}
              onToken={handleToken}
              onUnavailable={() => setIsUnavailable(true)}
            />
          )
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
