import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";

const codeLength = 6;

type TwoFactorSetupPayload = {
  secret: string;
  otpauth_url: string;
};

type RecoveryCodesPayload = {
  recovery_codes: string[];
};

export type TwoFactorSetup = {
  secret: string;
  otpauthUrl: string;
};

export type TwoFactorSubmitState = "idle" | "verifying" | "invalid";

export function useTwoFactorSetup() {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [setup, setSetup] = useState<TwoFactorSetup | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadFailed, setIsLoadFailed] = useState(false);
  const [isDisabledBySite, setIsDisabledBySite] = useState(false);
  const [code, setCode] = useState("");
  const [submitState, setSubmitState] = useState<TwoFactorSubmitState>("idle");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const hasRequestedSetup = useRef(false);
  const isSubmitting = useRef(false);

  const loadSetup = useCallback(async () => {
    setIsLoading(true);
    setIsLoadFailed(false);
    setIsDisabledBySite(false);

    try {
      const payload = await apiRequest<TwoFactorSetupPayload>(
        "/api/v1/users/me/2fa/setup",
        { method: "POST" },
      );
      setSetup({
        secret: payload.secret,
        otpauthUrl: payload.otpauth_url,
      });
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.status === 401) {
          return;
        }
        if (error.code === "two_factor_already_enabled") {
          router.refresh();
          return;
        }
        if (error.code === "two_factor_disabled") {
          setIsDisabledBySite(true);
          toast.error(t("twoFactorSiteDisabled"));
          router.refresh();
          return;
        }
      }

      setIsLoadFailed(true);
      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("twoFactorSetupFailed"),
      );
    } finally {
      setIsLoading(false);
    }
  }, [router, t, tCommon]);

  useEffect(() => {
    if (hasRequestedSetup.current) {
      return;
    }
    hasRequestedSetup.current = true;
    void loadSetup();
  }, [loadSetup]);

  const retrySetup = useCallback(() => {
    void loadSetup();
  }, [loadSetup]);

  const submitCode = useCallback(
    async (submittedCode: string) => {
      if (
        setup === null ||
        isSubmitting.current ||
        submittedCode.length !== codeLength
      ) {
        return;
      }

      isSubmitting.current = true;
      setSubmitState("verifying");

      try {
        const payload = await apiRequest<RecoveryCodesPayload>(
          "/api/v1/users/me/2fa/enable",
          { method: "POST", body: { code: submittedCode } },
        );
        setSubmitState("idle");
        setRecoveryCodes(payload.recovery_codes);
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.code === "two_factor_invalid_code") {
            setSubmitState("invalid");
            return;
          }
          if (error.status === 401) {
            return;
          }
          if (error.code === "two_factor_already_enabled") {
            setSubmitState("idle");
            router.refresh();
            return;
          }
          if (error.code === "two_factor_disabled") {
            setSubmitState("idle");
            setIsDisabledBySite(true);
            toast.error(t("twoFactorSiteDisabled"));
            router.refresh();
            return;
          }
          if (error.code === "two_factor_not_setup") {
            setSubmitState("idle");
            void loadSetup();
            return;
          }
        }

        setSubmitState("idle");
        toast.error(
          looksLikeServiceFailure(error)
            ? tCommon("serviceUnavailable")
            : t("twoFactorVerifyFailed"),
        );
      } finally {
        isSubmitting.current = false;
      }
    },
    [loadSetup, router, setup, t, tCommon],
  );

  const changeCode = useCallback(
    (value: string) => {
      const digits = value.replace(/\D/g, "").slice(0, codeLength);
      setCode(digits);
      setSubmitState("idle");

      if (digits.length === codeLength) {
        void submitCode(digits);
      }
    },
    [submitCode],
  );

  const finishRecovery = useCallback(() => {
    setRecoveryCodes(null);
    router.refresh();
  }, [router]);

  const isBusy = isLoading || submitState === "verifying";

  return {
    setup,
    isLoading,
    isLoadFailed,
    isDisabledBySite,
    code,
    submitState,
    recoveryCodes,
    isBusy,
    retrySetup,
    submitCode,
    changeCode,
    finishRecovery,
  };
}
