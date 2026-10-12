import { useCallback, useEffect } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { isServiceUp, looksLikeServiceFailure } from "@/lib/api";

export function useServiceErrorMessage() {
  const tCommon = useTranslations("common");

  return useCallback(
    async (error?: unknown): Promise<string | null> => {
      if (error !== undefined && !looksLikeServiceFailure(error)) {
        return null;
      }

      return (await isServiceUp()) ? null : tCommon("serviceUnavailable");
    },
    [tCommon],
  );
}

export function useServiceCheck() {
  const resolveServiceError = useServiceErrorMessage();

  useEffect(() => {
    let active = true;

    resolveServiceError().then((message) => {
      if (active && message) {
        toast.error(message);
      }
    });

    return () => {
      active = false;
    };
  }, [resolveServiceError]);
}
