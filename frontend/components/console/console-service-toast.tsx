"use client";

import { useEffect, useRef } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

const CONSOLE_SERVICE_TOAST_ID = "console-service-unavailable";

export function ConsoleServiceToast() {
  const t = useTranslations("common");
  const message = t("serviceUnavailable");
  const shown = useRef(false);

  useEffect(() => {
    if (shown.current) {
      return;
    }
    shown.current = true;
    toast.error(message, { id: CONSOLE_SERVICE_TOAST_ID });
  }, [message]);

  return null;
}
