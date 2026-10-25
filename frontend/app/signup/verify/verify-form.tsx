"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { AuthFrame } from "@/components/auth/auth-frame";
import { Button } from "@/components/ui/button";
import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import { markSignupSuccess } from "@/lib/signup-notice";

export function VerifyForm({ token }: { token: string }) {
  const t = useTranslations("auth.signup");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [isPending, setIsPending] = useState(false);
  const missingToken = token === "";

  useEffect(() => {
    if (missingToken) {
      toast.error(t("verifyInvalid"));
    }
  }, [missingToken, t]);

  async function verify() {
    if (isPending || missingToken) {
      return;
    }

    setIsPending(true);
    try {
      await apiRequest("/api/v1/users/email-verifications", {
        method: "POST",
        body: { token },
      });
      markSignupSuccess();
      router.push("/signin");
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.code === "email_verification_invalid") {
          toast.error(t("verifyInvalid"));
          return;
        }
        if (
          error.code === "email_taken" ||
          error.code === "nickname_taken" ||
          error.code === "already_taken"
        ) {
          toast.error(t("verifyTaken"));
          return;
        }
      }

      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("verifyFailed"),
      );
    } finally {
      setIsPending(false);
    }
  }

  return (
    <AuthFrame>
      <div className="flex flex-col gap-4">
        <h1 className="text-xl font-semibold">{t("verifyTitle")}</h1>

        {missingToken ? (
          <Link
            href="/signup"
            className="text-sm font-medium text-primary underline-offset-4 hover:underline"
          >
            {t("verifyGoSignup")}
          </Link>
        ) : (
          <Button
            type="button"
            disabled={isPending}
            onClick={() => void verify()}
          >
            {t("verifyAction")}
          </Button>
        )}
      </div>
    </AuthFrame>
  );
}
