"use client";

import { useEffect, useRef } from "react";
import { useTranslations } from "next-intl";

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
  const closeTimerRef = useRef<number | null>(null);

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

  function handleToken(token: string) {
    if (token === "" || closeTimerRef.current !== null) {
      return;
    }
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null;
      onSolved(token);
    }, CLOSE_DELAY_MS);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange} disablePointerDismissal>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("captchaDialogTitle")}</DialogTitle>
        </DialogHeader>
        {open ? (
          <TurnstileCaptcha siteKey={siteKey} onToken={handleToken} />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
