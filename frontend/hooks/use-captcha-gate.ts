import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, looksLikeServiceFailure } from "@/lib/api";
import {
  requestCaptchaChallenge,
  type CaptchaChallenge,
} from "@/lib/captcha-challenge";
import type { PublicCaptchaConfig } from "@/lib/public-settings";

export function useCaptchaGate(config: PublicCaptchaConfig | null) {
  const t = useTranslations("auth");
  const tCommon = useTranslations("common");
  const [isOpen, setIsOpen] = useState(false);
  const [challenge, setChallenge] = useState<CaptchaChallenge | null>(null);
  const preparingRef = useRef(false);
  const controllerRef = useRef<AbortController | null>(null);
  const isOpenRef = useRef(false);
  const disposedRef = useRef(false);

  useEffect(() => {
    disposedRef.current = false;
    return () => {
      disposedRef.current = true;
      controllerRef.current?.abort();
    };
  }, []);

  const begin = useCallback(() => {
    if (config === null || isOpenRef.current || preparingRef.current) {
      return;
    }

    if (config.provider === "turnstile") {
      isOpenRef.current = true;
      setIsOpen(true);
      return;
    }

    preparingRef.current = true;
    const controller = new AbortController();
    controllerRef.current = controller;

    void requestCaptchaChallenge(config.scope, controller.signal)
      .then((next) => {
        if (disposedRef.current || controller.signal.aborted) {
          return;
        }
        setChallenge(next);
        isOpenRef.current = true;
        setIsOpen(true);
      })
      .catch((error: unknown) => {
        if (disposedRef.current || controller.signal.aborted) {
          return;
        }
        if (error instanceof ApiError && error.status === 429) {
          toast.error(t("captchaRateLimited"));
          return;
        }
        toast.error(
          looksLikeServiceFailure(error)
            ? tCommon("serviceUnavailable")
            : t("captchaLoadFailed"),
        );
      })
      .finally(() => {
        if (controllerRef.current === controller) {
          controllerRef.current = null;
          preparingRef.current = false;
        }
      });
  }, [config, t, tCommon]);

  const cancel = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    preparingRef.current = false;
    isOpenRef.current = false;
    setChallenge(null);
    setIsOpen(false);
  }, []);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (open) {
        isOpenRef.current = true;
        setIsOpen(true);
        return;
      }
      cancel();
    },
    [cancel],
  );

  return { isOpen, challenge, begin, cancel, handleOpenChange };
}
