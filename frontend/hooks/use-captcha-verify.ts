import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";

export function useCaptchaVerify() {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const [isVerifying, setIsVerifying] = useState(false);
  const verifyingRef = useRef(false);

  async function verify(secretKey: string, token: string) {
    if (verifyingRef.current) {
      return;
    }

    verifyingRef.current = true;
    setIsVerifying(true);
    try {
      await apiRequest("/api/v1/admin/captcha/verifications", {
        method: "POST",
        body: { secret_key: secretKey, token },
      });
      toast.success(t("captchaVerifySucceeded"));
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        toast.error(t("captchaVerifyForbidden"));
        return;
      }
      if (
        error instanceof ApiError &&
        error.code === "captcha_verification_failed"
      ) {
        toast.error(t("captchaVerifyFailed"));
        return;
      }
      if (looksLikeServiceFailure(error)) {
        toast.error(tCommon("serviceUnavailable"));
        return;
      }
      toast.error(t("captchaVerifyFailed"));
    } finally {
      verifyingRef.current = false;
      setIsVerifying(false);
    }
  }

  return { isVerifying, verify };
}
