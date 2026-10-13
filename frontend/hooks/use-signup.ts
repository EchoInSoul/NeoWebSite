import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { useServiceHealth } from "@/hooks/use-service-health";
import { ApiError, apiRequest } from "@/lib/api";

export const PASSWORD_MIN_LENGTH = 8;
const NAME_MIN_LENGTH = 2;
const NAME_MAX_LENGTH = 32;
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
  const { serviceDown, resolveServiceError } = useServiceHealth();
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [isPending, setIsPending] = useState(false);

  function changeField(field: SignupField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  function toFailure(error: unknown): { message: string; field?: SignupField } {
    if (error instanceof ApiError) {
      switch (error.code) {
        case "email_taken":
          return { message: t("emailTaken"), field: "email" };
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
      }
    }

    return { message: t("failed") };
  }

  async function submit(): Promise<SignupField | undefined> {
    const email = values.email.trim();
    const nickname = values.name.trim();

    if (!isValidEmail(email)) {
      toast.error(t("invalidEmail"));
      return "email";
    }
    if (!isValidName(nickname)) {
      toast.error(
        t("invalidName", {
          minLength: NAME_MIN_LENGTH,
          maxLength: NAME_MAX_LENGTH,
        }),
      );
      return "name";
    }
    if (!isValidPassword(values.password)) {
      toast.error(t("weakPassword"));
      return "password";
    }
    if (values.confirmPassword !== values.password) {
      toast.error(t("mismatch"));
      return "confirmPassword";
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/users", {
        method: "POST",
        body: { email, nickname, password: values.password },
      });
      toast.success(t("success"));
      router.push("/console");
      return undefined;
    } catch (error) {
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

  return { values, isPending, serviceDown, changeField, submit };
}
