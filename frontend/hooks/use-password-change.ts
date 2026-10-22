import { useCallback, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  PASSWORD_MAX_BYTES,
  PASSWORD_MIN_LENGTH,
  isPasswordTooLong,
  isStrongPassword,
} from "@/lib/password";

export function usePasswordChange() {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const isSubmittingRef = useRef(false);

  const changeCurrentPassword = useCallback((value: string) => {
    setCurrentPassword(value);
  }, []);

  const changeNewPassword = useCallback((value: string) => {
    setNewPassword(value);
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
      toast.error(t("passwordTooLong", { maxBytes: PASSWORD_MAX_BYTES }));
      return;
    }

    if (!isStrongPassword(newPassword)) {
      toast.error(t("passwordWeak", { minLength: PASSWORD_MIN_LENGTH }));
      return;
    }

    isSubmittingRef.current = true;
    setIsSubmitting(true);

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
      toast.success(t("passwordChanged"));
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "invalid_credentials") {
          toast.error(t("invalidPassword"));
          return;
        }
        if (error.code === "weak_password") {
          toast.error(
            isPasswordTooLong(newPassword)
              ? t("passwordTooLong", { maxBytes: PASSWORD_MAX_BYTES })
              : t("passwordWeak", { minLength: PASSWORD_MIN_LENGTH }),
          );
          return;
        }
        if (error.code === "password_unchanged") {
          toast.error(t("passwordUnchanged"));
          return;
        }
        if (error.status === 401) {
          return;
        }
      }

      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("passwordChangeFailed"),
      );
    } finally {
      isSubmittingRef.current = false;
      setIsSubmitting(false);
    }
  }, [currentPassword, newPassword, t, tCommon]);

  return {
    currentPassword,
    newPassword,
    isSubmitting,
    changeCurrentPassword,
    changeNewPassword,
    submit,
  };
}
