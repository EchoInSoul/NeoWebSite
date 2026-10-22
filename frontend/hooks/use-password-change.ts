import { useCallback, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import { isPasswordTooLong, isStrongPassword } from "@/lib/password";

export type PasswordChangeState =
  "idle" | "submitting" | "invalidPassword" | "weak" | "tooLong" | "unchanged";

export function usePasswordChange() {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [state, setState] = useState<PasswordChangeState>("idle");
  const isSubmittingRef = useRef(false);

  const changeCurrentPassword = useCallback((value: string) => {
    setCurrentPassword(value);
    setState("idle");
  }, []);

  const changeNewPassword = useCallback((value: string) => {
    setNewPassword(value);
    setState("idle");
  }, []);

  const submit = useCallback(async () => {
    if (
      currentPassword === "" ||
      newPassword === "" ||
      isSubmittingRef.current
    ) {
      return;
    }

    if (isPasswordTooLong(newPassword)) {
      setState("tooLong");
      return;
    }

    if (!isStrongPassword(newPassword)) {
      setState("weak");
      return;
    }

    isSubmittingRef.current = true;
    setState("submitting");

    try {
      await apiRequest("/api/v1/users/me/password", {
        method: "POST",
        body: {
          current_password: currentPassword,
          new_password: newPassword,
        },
      });
      setCurrentPassword("");
      setNewPassword("");
      setState("idle");
      toast.success(t("passwordChanged"));
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "invalid_credentials") {
          setState("invalidPassword");
          return;
        }
        if (error.code === "weak_password") {
          setState(isPasswordTooLong(newPassword) ? "tooLong" : "weak");
          return;
        }
        if (error.code === "password_unchanged") {
          setState("unchanged");
          return;
        }
        if (error.status === 401) {
          setState("idle");
          return;
        }
      }

      setState("idle");
      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("passwordChangeFailed"),
      );
    } finally {
      isSubmittingRef.current = false;
    }
  }, [currentPassword, newPassword, t, tCommon]);

  return {
    currentPassword,
    newPassword,
    state,
    isSubmitting: state === "submitting",
    changeCurrentPassword,
    changeNewPassword,
    submit,
  };
}
