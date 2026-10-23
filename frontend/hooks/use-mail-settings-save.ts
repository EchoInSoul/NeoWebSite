import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  buildMailSettingsPatch,
  hasMailSettingsChanges,
  parseMailSettingsValues,
  validateMailSettings,
  type MailSettingsIssue,
  type MailSettingsValues,
} from "@/lib/mail-settings";

export function useMailSettingsSave(initial: MailSettingsValues) {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [values, setValues] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [isSaving, setIsSaving] = useState(false);

  const isDirty = hasMailSettingsChanges(values, baseline);

  function issueMessage(issue: MailSettingsIssue): string {
    switch (issue) {
      case "smtpPortRange":
        return t("mailSmtpPortRange");
      case "smtpEncryptionInvalid":
        return t("mailSmtpEncryptionInvalid");
      case "smtpPasswordRequired":
        return t("mailSmtpPasswordRequired");
      case "senderAddressInvalid":
        return t("mailSenderAddressInvalid");
    }
  }

  function conflictMessage(error: ApiError): string {
    switch (error.param) {
      case "smtp_port":
        return t("mailSmtpPortRange");
      case "smtp_encryption":
        return t("mailSmtpEncryptionInvalid");
      case "smtp_password":
        return t("mailSmtpPasswordRequired");
      case "sender_address":
        return t("mailSenderAddressInvalid");
      default:
        return t("saveFailed");
    }
  }

  function update<K extends keyof MailSettingsValues>(
    field: K,
    value: MailSettingsValues[K],
  ) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  async function save(): Promise<boolean> {
    if (isSaving) {
      return false;
    }
    if (!isDirty) {
      return true;
    }

    const issue = validateMailSettings(values, baseline);

    if (issue !== null) {
      toast.error(issueMessage(issue));
      return false;
    }

    setIsSaving(true);
    try {
      const payload = await apiRequest("/api/v1/admin/settings/mail", {
        method: "PATCH",
        body: buildMailSettingsPatch(values, baseline),
      });
      const parsed = parseMailSettingsValues(payload);

      if (parsed === null) {
        toast.error(t("saveFailed"));
        return false;
      }

      setValues(parsed);
      setBaseline(parsed);
      toast.success(t("saved"));
      router.refresh();
      return true;
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        return false;
      }
      if (error instanceof ApiError && error.status === 403) {
        toast.error(t("noPermissionToSave"));
        return false;
      }
      if (error instanceof ApiError && error.code === "setting_conflict") {
        toast.error(conflictMessage(error));
        return false;
      }
      if (looksLikeServiceFailure(error)) {
        toast.error(tCommon("serviceUnavailable"));
        return false;
      }
      toast.error(t("saveFailed"));
      return false;
    } finally {
      setIsSaving(false);
    }
  }

  return {
    values,
    isDirty,
    isSaving,
    update,
    save,
  };
}
