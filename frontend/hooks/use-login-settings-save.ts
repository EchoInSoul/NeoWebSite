import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  buildLoginSettingsPatch,
  hasLoginSettingsChanges,
  parseLoginGroupValues,
  validateLoginSettings,
  type LoginSettingsIssue,
  type LoginSettingsValues,
} from "@/lib/login-settings";

export function useLoginSettingsSave(initial: LoginSettingsValues) {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [values, setValues] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [isSaving, setIsSaving] = useState(false);

  const isDirty = hasLoginSettingsChanges(values, baseline);

  function issueMessage(issue: LoginSettingsIssue): string {
    switch (issue) {
      case "sessionMaxAgeDaysRange":
        return t("sessionMaxAgeDaysRange");
    }
  }

  function conflictMessage(error: ApiError): string {
    switch (error.param) {
      case "session_max_age_days":
        return t("sessionMaxAgeDaysRange");
      default:
        return t("saveFailed");
    }
  }

  function setSessionMaxAgeEnabled(checked: boolean) {
    setValues((previous) => ({
      ...previous,
      sessionMaxAgeEnabled: checked,
    }));
  }

  function updateSessionMaxAgeDays(value: string) {
    setValues((previous) => ({ ...previous, sessionMaxAgeDays: value }));
  }

  function setTotpEnabled(checked: boolean) {
    setValues((previous) => ({ ...previous, totpEnabled: checked }));
  }

  function setPasswordResetEnabled(checked: boolean) {
    setValues((previous) => ({
      ...previous,
      passwordResetEnabled: checked,
    }));
  }

  function setCaptchaEnabled(checked: boolean) {
    setValues((previous) => ({ ...previous, captchaEnabled: checked }));
  }

  function setCaptchaProvider(value: string) {
    setValues((previous) => ({ ...previous, captchaProvider: value }));
  }

  function setTurnstileSiteKey(value: string) {
    setValues((previous) => ({ ...previous, turnstileSiteKey: value }));
  }

  function setTurnstileSecretKey(value: string) {
    setValues((previous) => ({ ...previous, turnstileSecretKey: value }));
  }

  async function save() {
    if (isSaving || !isDirty) {
      return;
    }

    const issue = validateLoginSettings(values, baseline);

    if (issue !== null) {
      toast.error(issueMessage(issue));
      return;
    }

    setIsSaving(true);
    try {
      const payload = await apiRequest("/api/v1/admin/settings/auth", {
        method: "PATCH",
        body: buildLoginSettingsPatch(values, baseline),
      });
      const parsed = parseLoginGroupValues(payload);

      if (parsed === null) {
        toast.error(t("saveFailed"));
        return;
      }

      setValues(parsed);
      setBaseline(parsed);
      toast.success(t("saved"));
      router.refresh();
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        return;
      }
      if (error instanceof ApiError && error.status === 403) {
        toast.error(t("noPermissionToSave"));
        return;
      }
      if (error instanceof ApiError && error.code === "setting_conflict") {
        toast.error(conflictMessage(error));
        return;
      }
      if (looksLikeServiceFailure(error)) {
        toast.error(tCommon("serviceUnavailable"));
        return;
      }
      toast.error(t("saveFailed"));
    } finally {
      setIsSaving(false);
    }
  }

  return {
    values,
    isDirty,
    isSaving,
    setSessionMaxAgeEnabled,
    updateSessionMaxAgeDays,
    setTotpEnabled,
    setPasswordResetEnabled,
    setCaptchaEnabled,
    setCaptchaProvider,
    setTurnstileSiteKey,
    setTurnstileSecretKey,
    save,
  };
}
