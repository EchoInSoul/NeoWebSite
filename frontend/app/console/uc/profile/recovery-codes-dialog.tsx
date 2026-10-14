"use client";

import { Check, Copy } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

export function RecoveryCodesDialog({
  codes,
  onClose,
}: {
  codes: string[] | null;
  onClose: () => void;
}) {
  const t = useTranslations("uc");
  const { isCopied, copy } = useCopyToClipboard();

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
        <div className="flex flex-col items-start gap-3">
          <div className="grid w-full grid-cols-2 gap-x-4 gap-y-1.5 rounded-lg border border-border bg-muted/50 p-3 font-mono text-xs">
            {(codes ?? []).map((code) => (
              <span key={code}>{code}</span>
            ))}
          </div>
          <Button
            onClick={() => {
              void copy((codes ?? []).join("\n"));
            }}
            size="sm"
            type="button"
            variant="outline"
          >
            {isCopied ? <Check /> : <Copy />}
            {isCopied ? t("twoFactorCopied") : t("twoFactorRecoveryCopy")}
          </Button>
        </div>
        <AlertDialogFooter>
          <AlertDialogAction>{t("twoFactorRecoveryDone")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
