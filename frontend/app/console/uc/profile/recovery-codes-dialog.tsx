"use client";

import { useEffect, useRef, useState } from "react";
import { Download, Mail } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";

const autoDownloadDelayMs = 3000;

function downloadCodes(codes: string[]) {
  const blob = new Blob([codes.join("\n")], {
    type: "text/plain;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = "recovery-codes.txt";
  document.body.append(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function RecoveryCodesDialog({
  codes,
  onClose,
}: {
  codes: string[] | null;
  onClose: () => void;
}) {
  const t = useTranslations("uc");
  const tCommon = useTranslations("common");
  const [isSendingEmail, setIsSendingEmail] = useState(false);
  const autoDownloadTimer = useRef<number | null>(null);

  useEffect(() => {
    if (codes === null) {
      return;
    }

    autoDownloadTimer.current = window.setTimeout(() => {
      autoDownloadTimer.current = null;
      downloadCodes(codes);
    }, autoDownloadDelayMs);

    return () => {
      if (autoDownloadTimer.current !== null) {
        window.clearTimeout(autoDownloadTimer.current);
        autoDownloadTimer.current = null;
      }
    };
  }, [codes]);

  function handleDownload() {
    if (codes === null) {
      return;
    }

    if (autoDownloadTimer.current !== null) {
      window.clearTimeout(autoDownloadTimer.current);
      autoDownloadTimer.current = null;
    }
    downloadCodes(codes);
  }

  async function handleSendEmail() {
    if (isSendingEmail) {
      return;
    }

    setIsSendingEmail(true);

    try {
      await apiRequest("/api/v1/users/me/2fa/recovery-codes/email", {
        method: "POST",
      });
      toast.success(t("twoFactorRecoveryEmailed"));
    } catch (error) {
      if (error instanceof ApiError) {
        if (error.status === 401) {
          return;
        }
        if (error.code === "not_implemented") {
          toast.error(t("twoFactorRecoveryEmailUnavailable"));
          return;
        }
      }

      toast.error(
        looksLikeServiceFailure(error)
          ? tCommon("serviceUnavailable")
          : t("twoFactorRecoveryEmailFailed"),
      );
    } finally {
      setIsSendingEmail(false);
    }
  }

  return (
    <AlertDialog
      open={codes !== null}
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <AlertDialogContent className="sm:max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("twoFactorRecoveryTitle")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("twoFactorRecoveryHint")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <Button
            disabled={isSendingEmail}
            onClick={() => void handleSendEmail()}
            type="button"
            variant="outline"
          >
            <Mail />
            {t("twoFactorRecoveryEmail")}
          </Button>
          <Button onClick={handleDownload} type="button">
            <Download />
            {t("twoFactorRecoveryDownload")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
