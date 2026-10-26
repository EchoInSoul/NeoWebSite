import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";

export type MailTestTemplate =
  "signup_code" | "password_reset" | "recovery_codes";

export type MailTestTarget = {
  template: MailTestTemplate;
  label: string;
};

export function useMailTestSend(recipient: string) {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const [target, setTarget] = useState<MailTestTarget | null>(null);
  const [isSending, setIsSending] = useState(false);
  const targetRef = useRef<MailTestTarget | null>(null);
  const sendingRef = useRef(false);

  function requestSend(next: MailTestTarget) {
    targetRef.current = next;
    setTarget(next);
  }

  function close() {
    if (!sendingRef.current) {
      targetRef.current = null;
      setTarget(null);
    }
  }

  async function confirmSend() {
    const current = targetRef.current;
    if (current === null || sendingRef.current) {
      return;
    }
    if (recipient === "") {
      toast.error(t("mailSendTestNoRecipient"));
      return;
    }

    sendingRef.current = true;
    setIsSending(true);
    try {
      await apiRequest("/api/v1/admin/mail/messages", {
        method: "POST",
        body: { to: recipient, template: current.template },
      });
      toast.success(t("mailSendTestSent", { email: recipient }));
      targetRef.current = null;
      setTarget(null);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        toast.error(t("mailSendTestForbidden"));
        targetRef.current = null;
        setTarget(null);
        return;
      }
      if (error instanceof ApiError && error.code === "mail_delivery_failed") {
        toast.error(t("mailSendTestFailed"));
        return;
      }
      if (looksLikeServiceFailure(error)) {
        toast.error(tCommon("serviceUnavailable"));
        return;
      }
      toast.error(t("mailSendTestFailed"));
    } finally {
      sendingRef.current = false;
      setIsSending(false);
    }
  }

  return {
    target,
    isSending,
    requestSend,
    close,
    confirmSend,
  };
}
