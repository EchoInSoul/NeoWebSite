import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import {
  consumeResetDone,
  consumeResetRequested,
} from "@/lib/password-reset-notice";

export function usePasswordResetNotice() {
  const t = useTranslations("auth.reset");

  useEffect(() => {
    if (consumeResetRequested()) {
      toast.success(t("sent"));
      return;
    }
    if (consumeResetDone()) {
      toast.success(t("success"));
    }
  }, [t]);
}
