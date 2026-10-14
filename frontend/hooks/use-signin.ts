import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { useServiceHealth } from "@/hooks/use-service-health";
import { ApiError, apiRequest } from "@/lib/api";

export type SigninField = "identifier" | "password";

export type SigninStep = "credentials" | "verify";

const EMPTY_VALUES: Record<SigninField, string> = {
  identifier: "",
  password: "",
};

export function useSigninForm() {
  const t = useTranslations("auth.signin");
  const { serviceDown, resolveServiceError } = useServiceHealth();
  const router = useRouter();
  const [values, setValues] = useState(EMPTY_VALUES);
  const [step, setStep] = useState<SigninStep>("credentials");
  const [code, setCode] = useState("");
  const [isPending, setIsPending] = useState(false);

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
      await apiRequest("/api/v1/sessions", {
        method: "POST",
        body: { identifier, password },
      });
      router.push("/console");
      return undefined;
    } catch (error) {
      if (error instanceof ApiError && error.code === "two_factor_required") {
        setValues((previous) => ({ ...previous, password: "" }));
        setCode("");
        setStep("verify");
        return undefined;
      }

      toast.error((await resolveServiceError(error)) ?? toMessage(error));
      return undefined;
    } finally {
      setIsPending(false);
    }
  }

  async function submitCode(): Promise<"code" | undefined> {
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
      router.push("/console");
      return undefined;
    } catch (error) {
      if (
        error instanceof ApiError &&
        (error.code === "two_factor_locked" || error.code === "unauthorized")
      ) {
        backToCredentials();
      }

      toast.error((await resolveServiceError(error)) ?? toMessage(error));
      return undefined;
    } finally {
      setIsPending(false);
    }
  }

  return {
    values,
    step,
    code,
    isPending,
    serviceDown,
    changeField,
    changeCode,
    backToCredentials,
    submit,
    submitCode,
  };
}
