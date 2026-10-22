import { useEffect } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { consumeSignoutSuccess } from "@/lib/signout-notice";

export function useSignoutNotice() {
  const t = useTranslations("userMenu");

  useEffect(() => {
    if (consumeSignoutSuccess()) {
      toast.success(t("signedOut"));
    }
  }, [t]);
}
