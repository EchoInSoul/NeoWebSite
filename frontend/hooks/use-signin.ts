import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";

import { ApiError, apiRequest } from "@/lib/api";

export type SigninField = "identifier" | "password";

export type SigninErrors = Partial<Record<SigninField | "form", string>>;

const EMPTY_VALUES: Record<SigninField, string> = {
  identifier: "",
  password: "",
};

export function useSigninForm() {
  const t = useTranslations("auth.signin");
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [errors, setErrors] = useState<SigninErrors>({});
  const [isPending, setIsPending] = useState(false);

  function changeField(field: SigninField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
    setErrors((previous) =>
      previous[field] === undefined
        ? previous
        : { ...previous, [field]: undefined },
    );
  }

  function toFieldErrors(error: unknown): SigninErrors {
    if (!(error instanceof ApiError)) {
      return { form: t("failed") };
    }

    switch (error.code) {
      case "invalid_credentials":
        return { form: t("invalidCredentials") };
      case "two_factor_required":
        return { form: t("twoFactorUnsupported") };
      default:
        return { form: t("failed") };
    }
  }

  async function submit(): Promise<SigninField | undefined> {
    const identifier = values.identifier.trim();
    const password = values.password;

    const nextErrors: SigninErrors = {};
    if (identifier === "") {
      nextErrors.identifier = t("requiredIdentifier");
    }
    if (password === "") {
      nextErrors.password = t("requiredPassword");
    }
    setErrors(nextErrors);

    if (nextErrors.identifier) {
      return "identifier";
    }
    if (nextErrors.password) {
      return "password";
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/auth/login", {
        method: "POST",
        body: { identifier, password },
      });
      router.push("/console");
      return undefined;
    } catch (error) {
      setErrors(toFieldErrors(error));
      return undefined;
    } finally {
      setIsPending(false);
    }
  }

  return { values, errors, isPending, changeField, submit };
}
