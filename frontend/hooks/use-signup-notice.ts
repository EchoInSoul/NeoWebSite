import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import {
  consumeSignupPending,
  consumeSignupSuccess,
} from "@/lib/signup-notice";

export function useSignupNotice() {
  const t = useTranslations("auth.signup");

  useEffect(() => {
    if (consumeSignupPending()) {
      toast.success(t("verificationSent"));
      return;
    }
    if (consumeSignupSuccess()) {
      toast.success(t("success"));
    }
  }, [t]);
}
