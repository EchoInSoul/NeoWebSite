import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { toast } from "sonner";

import { useServiceHealth } from "@/hooks/use-service-health";
import { ApiError, apiRequest } from "@/lib/api";
import { isStrongPassword } from "@/lib/password";
import { formatRetryDuration } from "@/lib/rate-limit";
import { markSignupSuccess } from "@/lib/signup-notice";

const NAME_MIN_LENGTH = 2;
const NAME_MAX_LENGTH = 32;
const NAME_PATTERN = /^[\p{L}\p{N}_-]+$/u;
const EMAIL_MAX_LENGTH = 254;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type SignupField = "email" | "name" | "password" | "confirmPassword";

const EMPTY_VALUES: Record<SignupField, string> = {
  email: "",
  name: "",
  password: "",
  confirmPassword: "",
};

function isValidEmail(value: string) {
  return value.length <= EMAIL_MAX_LENGTH && EMAIL_PATTERN.test(value);
}

function isValidName(value: string) {
  const length = [...value].length;
  return (
    length >= NAME_MIN_LENGTH &&
    length <= NAME_MAX_LENGTH &&
    NAME_PATTERN.test(value)
  );
}

export function useSignupForm(
  captchaRequired: boolean,
  onCaptchaRequired: () => void,
) {
  const t = useTranslations("auth.signup");
  const locale = useLocale();
  const { serviceDown, resolveServiceError } = useServiceHealth();
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
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

  function changeField(field: SignupField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  function toFailure(error: unknown): { message: string; field?: SignupField } {
    if (error instanceof ApiError) {
      switch (error.code) {
        case "email_taken":
          return { message: t("emailTaken"), field: "email" };
        case "email_domain_not_allowed":
          return { message: t("emailDomainNotAllowed"), field: "email" };
        case "nickname_taken":
          return { message: t("nameTaken"), field: "name" };
        case "weak_password":
          return { message: t("weakPassword"), field: "password" };
        case "already_taken":
          return { message: t("alreadyTaken") };
        case "invalid_request":
          return { message: t("invalidRequest") };
        case "registration_closed":
          return { message: t("registrationClosed") };
        case "rate_limited":
          return { message: t("rateLimited") };
        case "captcha_failed":
          return { message: t("captchaFailed") };
      }
    }

    return { message: t("failed") };
  }

  async function submit(captchaToken = ""): Promise<SignupField | undefined> {
    if (retryRemaining > 0) {
      return undefined;
    }

    const email = values.email.trim();
    const nickname = values.name.trim();

    if (nickname === "") {
      toast.error(t("requiredName"));
      return "name";
    }
    if (!isValidName(nickname)) {
      toast.error(
        t("invalidName", {
          minLength: NAME_MIN_LENGTH,
        }),
      );
      return "name";
    }
    if (email === "") {
      toast.error(t("requiredEmail"));
      return "email";
    }
    if (!isValidEmail(email)) {
      toast.error(t("invalidEmail"));
      return "email";
    }
    if (values.password === "") {
      toast.error(t("requiredPassword"));
      return "password";
    }
    if (!isStrongPassword(values.password)) {
      toast.error(t("weakPassword"));
      return "password";
    }
    if (values.confirmPassword === "") {
      toast.error(t("requiredConfirmPassword"));
      return "confirmPassword";
    }
    if (values.confirmPassword !== values.password) {
      toast.error(t("mismatch"));
      return "confirmPassword";
    }
    if (captchaRequired && captchaToken === "") {
      onCaptchaRequired();
      return undefined;
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/users", {
        method: "POST",
        body: {
          email,
          nickname,
          password: values.password,
          ...(captchaToken === "" ? {} : { captcha_token: captchaToken }),
        },
      });
      markSignupSuccess();
      setValues(EMPTY_VALUES);
      router.push("/signin");
      return undefined;
    } catch (error) {
      if (error instanceof ApiError && error.code === "rate_limited") {
        applyRateLimit(error.retryAfter ?? 60);
      }

      const serviceError = await resolveServiceError(error);
      if (serviceError) {
        toast.error(serviceError);
        return undefined;
      }

      const failure = toFailure(error);
      toast.error(failure.message);
      return failure.field;
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
    changeField,
    submit,
  };
}
