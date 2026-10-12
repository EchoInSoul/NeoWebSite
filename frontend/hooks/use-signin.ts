import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest } from "@/lib/api";

export type SigninField = "identifier" | "password";

const EMPTY_VALUES: Record<SigninField, string> = {
  identifier: "",
  password: "",
};

export function useSigninForm() {
  const t = useTranslations("auth.signin");
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [isPending, setIsPending] = useState(false);

  function changeField(field: SigninField, value: string) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  function toMessage(error: unknown) {
    if (error instanceof ApiError) {
      switch (error.code) {
        case "invalid_credentials":
          return t("invalidCredentials");
        case "two_factor_required":
          return t("twoFactorUnsupported");
      }
    }

    return t("failed");
  }

  async function submit(): Promise<SigninField | undefined> {
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
      await apiRequest("/api/v1/auth/login", {
        method: "POST",
        body: { identifier, password },
      });
      router.push("/console");
      return undefined;
    } catch (error) {
      toast.error(toMessage(error));
      return undefined;
    } finally {
      setIsPending(false);
    }
  }

  return { values, isPending, changeField, submit };
}
