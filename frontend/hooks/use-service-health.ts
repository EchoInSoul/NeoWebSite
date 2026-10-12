import { useCallback, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { isServiceUp, looksLikeServiceFailure } from "@/lib/api";

export function useServiceHealth() {
  const tCommon = useTranslations("common");
  const unavailableMessage = tCommon("serviceUnavailable");
  const [serviceDown, setServiceDown] = useState(false);

  const resolveServiceError = useCallback(
    async (error?: unknown): Promise<string | null> => {
      if (error !== undefined && !looksLikeServiceFailure(error)) {
        setServiceDown(false);
        return null;
      }

      const up = await isServiceUp();
      setServiceDown(!up);
      return up ? null : unavailableMessage;
    },
    [unavailableMessage],
  );

  useEffect(() => {
    let active = true;

    isServiceUp().then((up) => {
      if (!active) {
        return;
      }

      setServiceDown(!up);
      if (!up) {
        toast.error(unavailableMessage);
      }
    });

    return () => {
      active = false;
    };
  }, [unavailableMessage]);

  return { serviceDown, resolveServiceError };
}
