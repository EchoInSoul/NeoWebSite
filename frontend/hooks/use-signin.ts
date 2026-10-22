import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { toast } from "sonner";

import { useServiceHealth } from "@/hooks/use-service-health";
import { ApiError, apiRequest } from "@/lib/api";
import { formatRetryDuration } from "@/lib/rate-limit";
import { markSigninSuccess } from "@/lib/signin-notice";

export type SigninField = "identifier" | "password";

export type SigninStep = "credentials" | "verify";

const EMPTY_VALUES: Record<SigninField, string> = {
  identifier: "",
  password: "",
};

export function useSigninForm() {
  const t = useTranslations("auth.signin");
  const locale = useLocale();
  const { serviceDown, resolveServiceError } = useServiceHealth();
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [step, setStep] = useState<SigninStep>("credentials");
  const [code, setCode] = useState("");
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

  function changeField(field: SigninField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  function changeCode(value: string) {
    setCode(value);
  }

  function backToCredentials() {
    setCode("");
    setStep("credentials");
  }

  function toMessage(error: unknown) {
    if (error instanceof ApiError) {
      switch (error.code) {
        case "invalid_credentials":
          return t("invalidCredentials");
        case "two_factor_invalid_code":
          return t("invalidCode");
        case "two_factor_locked":
          return t("locked");
        case "unauthorized":
          return t("verifyExpired");
        case "rate_limited":
          return t("rateLimited");
      }
    }

    return t("failed");
  }

  async function submit(): Promise<SigninField | undefined> {
    if (retryRemaining > 0) {
      return undefined;
    }

    const identifier = values.identifier.trim();
    const password = values.password;

    if (identifier === "") {
      toast.error(t("requiredIdentifier"));
      return "identifier";
    }
    if (password === "") {
      toast.error(t("requiredPassword"));
      return "password";
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/sessions", {
        method: "POST",
        body: { identifier, password },
      });
      markSigninSuccess();
      router.push("/console");
      return undefined;
    } catch (error) {
      if (error instanceof ApiError && error.code === "two_factor_required") {
        setValues((previous) => ({ ...previous, password: "" }));
        setCode("");
        setStep("verify");
        return undefined;
      }

      if (error instanceof ApiError && error.code === "rate_limited") {
        applyRateLimit(error.retryAfter ?? 60);
      }

      toast.error((await resolveServiceError(error)) ?? toMessage(error));
      return undefined;
    } finally {
      setIsPending(false);
    }
  }

  async function submitCode(): Promise<"code" | undefined> {
    if (retryRemaining > 0) {
      return undefined;
    }

    const trimmedCode = code.trim();

    if (trimmedCode === "") {
      toast.error(t("requiredCode"));
      return "code";
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/sessions/current/verify", {
        method: "POST",
        body: { code: trimmedCode },
      });
      markSigninSuccess();
      router.push("/console");
      return undefined;
    } catch (error) {
      if (
        error instanceof ApiError &&
        (error.code === "two_factor_locked" || error.code === "unauthorized")
      ) {
        backToCredentials();
      }

      if (error instanceof ApiError && error.code === "rate_limited") {
        applyRateLimit(error.retryAfter ?? 60);
      }

      toast.error((await resolveServiceError(error)) ?? toMessage(error));
      return undefined;
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
    step,
    code,
    isPending,
    serviceDown,
    retryRemaining,
    retryLabel,
    changeField,
    changeCode,
    backToCredentials,
    submit,
    submitCode,
  };
}
