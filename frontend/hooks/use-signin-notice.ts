import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { consumeSigninSuccess } from "@/lib/signin-notice";

export function useSigninNotice() {
  const t = useTranslations("auth.signin");

  useEffect(() => {
    if (consumeSigninSuccess()) {
      toast.success(t("success"));
    }
  }, [t]);
}
