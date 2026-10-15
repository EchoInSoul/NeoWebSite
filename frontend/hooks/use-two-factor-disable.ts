import { useCallback, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";

export function useTwoFactorDisable(onDisabled: () => void) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const isSubmittingRef = useRef(false);

  const submit = useCallback(async () => {
    if (password === "" || isSubmittingRef.current) {
      return;
    }

    isSubmittingRef.current = true;
    setIsSubmitting(true);

    try {
      await apiRequest("/api/v1/users/me/2fa/disable", {
        method: "POST",
        body: { password },
      });
      setPassword("");
      toast.success(t("twoFactorDisabled"));
      onDisabled();
      router.refresh();
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "invalid_credentials") {
          toast.error(t("invalidPassword"));
          return;
        }
        if (error.status === 401) {
          return;
        }
        if (error.code === "two_factor_not_setup") {
          onDisabled();
          router.refresh();
          return;
        }
      }

      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("twoFactorDisableFailed"),
      );
    } finally {
      isSubmittingRef.current = false;
      setIsSubmitting(false);
    }
  }, [onDisabled, password, router, t, tCommon]);

  return {
    password,
    isSubmitting,
    changePassword: setPassword,
    submit,
  };
}
