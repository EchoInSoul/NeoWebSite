import { useCallback, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";

export type TwoFactorDisableState = "idle" | "submitting" | "invalidPassword";

export function useTwoFactorDisable(onDisabled: () => void) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [state, setState] = useState<TwoFactorDisableState>("idle");
  const isSubmitting = useRef(false);

  const changePassword = useCallback((value: string) => {
    setPassword(value);
    setState("idle");
  }, []);

  const submit = useCallback(async () => {
    if (password === "" || isSubmitting.current) {
      return;
    }

    isSubmitting.current = true;
    setState("submitting");

    try {
      await apiRequest("/api/v1/users/me/2fa/disable", {
        method: "POST",
        body: { password },
      });
      setPassword("");
      setState("idle");
      toast.success(t("twoFactorDisabled"));
      onDisabled();
      router.refresh();
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "invalid_credentials") {
          setState("invalidPassword");
          return;
        }
        if (error.status === 401) {
          router.replace("/signin");
          return;
        }
        if (error.code === "two_factor_not_setup") {
          setState("idle");
          onDisabled();
          router.refresh();
          return;
        }
      }

      setState("idle");
      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("twoFactorDisableFailed"),
      );
    } finally {
      isSubmitting.current = false;
    }
  }, [onDisabled, password, router, t, tCommon]);

  return {
    password,
    state,
    isSubmitting: state === "submitting",
    changePassword,
    submit,
  };
}
