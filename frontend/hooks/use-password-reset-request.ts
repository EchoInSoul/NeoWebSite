import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { toast } from "sonner";

import { useServiceHealth } from "@/hooks/use-service-health";
import { ApiError, apiRequest } from "@/lib/api";
import { markResetRequested } from "@/lib/password-reset-notice";
import { formatRetryDuration } from "@/lib/rate-limit";

const EMAIL_MAX_LENGTH = 254;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function isValidEmail(value: string) {
  return value.length <= EMAIL_MAX_LENGTH && EMAIL_PATTERN.test(value);
}

export function usePasswordResetRequest() {
  const t = useTranslations("auth.reset");
  const locale = useLocale();
  const router = useRouter();
  const { serviceDown, resolveServiceError } = useServiceHealth();
  const [email, setEmail] = useState("");
  const [isPending, setIsPending] = useState(false);
  const [retryUntil, setRetryUntil] = useState(0);
  const [retryRemaining, setRetryRemaining] = useState(0);

  useEffect(() => {
    if (retryUntil === 0) {
      return;
    }

    const timer = window.setInterval(() => {
      const remaining = Math.max(
        0,
        Math.ceil((retryUntil - Date.now()) / 1000),
      );
      setRetryRemaining(remaining);
      if (remaining === 0) {
        setRetryUntil(0);
      }
    }, 1000);
    return () => window.clearInterval(timer);
  }, [retryUntil]);

  function applyRateLimit(seconds: number) {
    const safe = Math.max(1, Math.ceil(seconds));
    setRetryRemaining(safe);
    setRetryUntil(Date.now() + safe * 1000);
  }

  async function submit() {
    if (retryRemaining > 0) {
      return;
    }

    const value = email.trim();
    if (value === "") {
      toast.error(t("requiredEmail"));
      return;
    }
    if (!isValidEmail(value)) {
      toast.error(t("invalidEmail"));
      return;
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/users/password-resets", {
        method: "POST",
        body: { email: value },
      });
      markResetRequested();
      router.push("/login");
    } catch (error) {
      if (error instanceof ApiError && error.code === "rate_limited") {
        applyRateLimit(error.retryAfter ?? 60);
      }

      const serviceError = await resolveServiceError(error);
      if (serviceError) {
        toast.error(serviceError);
        return;
      }

      if (error instanceof ApiError) {
        if (error.code === "mail_unavailable") {
          toast.error(t("mailUnavailable"));
          return;
        }
        if (error.code === "password_reset_disabled") {
          toast.error(t("disabled"));
          return;
        }
        if (error.code === "rate_limited") {
          toast.error(t("rateLimited"));
          return;
        }
      }

      toast.error(t("failed"));
    } finally {
      setIsPending(false);
    }
  }

  const retryLabel =
    retryRemaining > 0
      ? t("retryIn", {
          duration: formatRetryDuration(retryRemaining, locale),
        })
      : null;

  return {
    email,
    isPending,
    serviceDown,
    retryRemaining,
    retryLabel,
    changeEmail: setEmail,
    submit,
  };
}
