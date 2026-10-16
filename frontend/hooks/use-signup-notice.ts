import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { consumeSignupSuccess } from "@/lib/signup-notice";

export function useSignupNotice() {
  const t = useTranslations("auth.signup");

  useEffect(() => {
    if (consumeSignupSuccess()) {
      toast.success(t("success"));
    }
  }, [t]);
}
