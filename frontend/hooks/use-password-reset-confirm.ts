import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { toast } from "sonner";

import { useServiceHealth } from "@/hooks/use-service-health";
import { ApiError, apiRequest } from "@/lib/api";
import { markResetDone } from "@/lib/password-reset-notice";
import { isStrongPassword } from "@/lib/password";
import { formatRetryDuration } from "@/lib/rate-limit";

export type ResetConfirmField = "password" | "confirmPassword";

const EMPTY_VALUES: Record<ResetConfirmField, string> = {
  password: "",
  confirmPassword: "",
};

export function usePasswordResetConfirm(token: string) {
  const t = useTranslations("auth.reset");
  const locale = useLocale();
  const router = useRouter();
  const { serviceDown, resolveServiceError } = useServiceHealth();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [isPending, setIsPending] = useState(false);
  const [retryUntil, setRetryUntil] = useState(0);
  const [retryRemaining, setRetryRemaining] = useState(0);
  const [tokenRejected, setTokenRejected] = useState(false);

  const missingToken = token === "";
  const invalidToken = missingToken || tokenRejected;

  useEffect(() => {
    if (missingToken) {
      toast.error(t("invalidToken"));
    }
  }, [missingToken, t]);

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

  function changeField(field: ResetConfirmField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  async function submit() {
    if (retryRemaining > 0 || invalidToken) {
      return;
    }

    if (values.password === "") {
      toast.error(t("requiredPassword"));
      return;
    }
    if (values.confirmPassword === "") {
      toast.error(t("requiredConfirmPassword"));
      return;
    }
    if (values.confirmPassword !== values.password) {
      toast.error(t("mismatch"));
      return;
    }
    if (!isStrongPassword(values.password)) {
      toast.error(t("weakPassword"));
      return;
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/users/password-resets/confirmations", {
        method: "POST",
        body: { token, new_password: values.password },
      });
      markResetDone();
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
        if (error.code === "password_reset_invalid") {
          setTokenRejected(true);
          toast.error(t("invalidToken"));
          return;
        }
        if (error.code === "weak_password") {
          toast.error(t("weakPassword"));
          return;
        }
        if (error.code === "password_unchanged") {
          toast.error(t("passwordUnchanged"));
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
    values,
    isPending,
    serviceDown,
    retryRemaining,
    retryLabel,
    invalidToken,
    changeField,
    submit,
  };
}
