import { useCallback, useRef, useState } from "react";
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
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [isLoadFailed, setIsLoadFailed] = useState(false);
  const [isDisabledBySite, setIsDisabledBySite] = useState(false);
  const [code, setCode] = useState("");
  const [submitState, setSubmitState] = useState<TwoFactorSubmitState>("idle");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const isStarting = useRef(false);
  const isSubmitting = useRef(false);

  const startSetup = useCallback(async () => {
    if (password === "" || isStarting.current) {
      return;
    }

    isStarting.current = true;
    setIsLoading(true);
    setIsLoadFailed(false);
    setIsDisabledBySite(false);

    try {
      const payload = await apiRequest<TwoFactorSetupPayload>(
        "/api/v1/users/me/2fa/setup",
        { method: "POST", body: { password } },
      );
      setSetup({
        secret: payload.secret,
        otpauthUrl: payload.otpauth_url,
      });
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "invalid_credentials") {
          toast.error(t("invalidPassword"));
          return;
        }
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
      isStarting.current = false;
      setIsLoading(false);
    }
  }, [password, router, t, tCommon]);

  const changePassword = useCallback((value: string) => {
    setPassword(value);
    setIsLoadFailed(false);
  }, []);

  const retrySetup = useCallback(() => {
    void startSetup();
  }, [startSetup]);

  const submitCode = useCallback(
    async (submittedCode: string) => {
      if (
        setup === null ||
        isSubmitting.current ||
        submittedCode.length !== codeLength ||
        password === ""
      ) {
        return;
      }

      isSubmitting.current = true;
      setSubmitState("verifying");

      try {
        const payload = await apiRequest<RecoveryCodesPayload>(
          "/api/v1/users/me/2fa/enable",
          { method: "POST", body: { code: submittedCode, password } },
        );
        setSubmitState("idle");
        setPassword("");
        setRecoveryCodes(payload.recovery_codes);
      } catch (error) {
        if (error instanceof ApiError) {
          if (error.code === "two_factor_invalid_code") {
            setSubmitState("invalid");
            return;
          }
          if (error.code === "invalid_credentials") {
            setSubmitState("idle");
            setSetup(null);
            setPassword("");
            toast.error(t("invalidPassword"));
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
            void startSetup();
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
    [password, router, setup, startSetup, t, tCommon],
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
    setPassword("");
    router.refresh();
  }, [router]);

  const isBusy = isLoading || submitState === "verifying";

  return {
    setup,
    password,
    isLoading,
    isLoadFailed,
    isDisabledBySite,
    code,
    submitState,
    recoveryCodes,
    isBusy,
    retrySetup,
    startSetup,
    changePassword,
    submitCode,
    changeCode,
    finishRecovery,
  };
}
