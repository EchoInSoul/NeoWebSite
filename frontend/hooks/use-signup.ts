import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest } from "@/lib/api";

export const PASSWORD_MIN_LENGTH = 8;
const NAME_MIN_LENGTH = 2;
const NAME_MAX_LENGTH = 32;
const EMAIL_MAX_LENGTH = 254;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type SignupField = "email" | "name" | "password" | "confirmPassword";

export type SignupErrors = Partial<Record<SignupField | "form", string>>;

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
    !/[\s@]/.test(value)
  );
}

function isValidPassword(value: string) {
  return (
    [...value].length >= PASSWORD_MIN_LENGTH &&
    /[a-zA-Z]/.test(value) &&
    /[0-9]/.test(value)
  );
}

export function useSignupForm() {
  const t = useTranslations("auth.signup");
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [errors, setErrors] = useState<SignupErrors>({});
  const [isPending, setIsPending] = useState(false);

  const confirmError =
    values.confirmPassword.length > 0 &&
    values.confirmPassword !== values.password
      ? t("mismatch")
      : undefined;

  function changeField(field: SignupField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
    setErrors((previous) =>
      previous[field] === undefined
        ? previous
        : { ...previous, [field]: undefined },
    );
  }

  function toFieldErrors(error: unknown): SignupErrors {
    if (!(error instanceof ApiError)) {
      return { form: t("failed") };
    }

    switch (error.code) {
      case "email_taken":
        return { email: t("emailTaken") };
      case "nickname_taken":
        return { name: t("nameTaken") };
      case "weak_password":
        return { password: t("weakPassword") };
      case "already_taken":
        return { form: t("alreadyTaken") };
      case "invalid_request":
        return { form: t("invalidRequest") };
      default:
        return { form: t("failed") };
    }
  }

  function firstErrorField(current: SignupErrors): SignupField | undefined {
    if (current.email) {
      return "email";
    }
    if (current.name) {
      return "name";
    }
    if (current.password) {
      return "password";
    }
    return undefined;
  }

  async function submit(): Promise<SignupField | undefined> {
    const email = values.email.trim();
    const nickname = values.name.trim();

    const nextErrors: SignupErrors = {};
    if (!isValidEmail(email)) {
      nextErrors.email = t("invalidEmail");
    }
    if (!isValidName(nickname)) {
      nextErrors.name = t("invalidName", {
        minLength: NAME_MIN_LENGTH,
        maxLength: NAME_MAX_LENGTH,
      });
    }
    if (!isValidPassword(values.password)) {
      nextErrors.password = t("weakPassword");
    }
    setErrors(nextErrors);

    const invalidField =
      firstErrorField(nextErrors) ??
      (confirmError ? "confirmPassword" : undefined);
    if (invalidField) {
      return invalidField;
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/auth/register", {
        method: "POST",
        body: { email, nickname, password: values.password },
      });
      toast.success(t("success"));
      router.push("/console");
      return undefined;
    } catch (error) {
      const apiErrors = toFieldErrors(error);
      setErrors(apiErrors);
      return firstErrorField(apiErrors);
    } finally {
      setIsPending(false);
    }
  }

  return { values, errors, confirmError, isPending, changeField, submit };
}
