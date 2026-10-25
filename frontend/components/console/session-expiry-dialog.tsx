"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { subscribeSessionExpired } from "@/lib/session-expiry";

export function SessionExpiryDialog() {
  const t = useTranslations("sessionExpiry");
  const [isOpen, setIsOpen] = useState(false);

  useEffect(() => {
    return subscribeSessionExpired(() => {
      setIsOpen(true);
    });
  }, []);

  function handleRelogin() {
    window.location.replace("/login");
  }

  return (
    <AlertDialog open={isOpen} onOpenChange={setIsOpen}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("title")}</AlertDialogTitle>
          <AlertDialogDescription>{t("description")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("stay")}</AlertDialogCancel>
          <AlertDialogAction onClick={handleRelogin}>
            {t("relogin")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
