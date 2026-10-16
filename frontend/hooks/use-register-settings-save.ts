import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  buildRegisterSettingsPatch,
  hasRegisterSettingsChanges,
  parseAuthGroupValues,
  parseRegisterGroupValues,
  validateRegisterSettings,
  type RegisterSettingsIssue,
  type RegisterSettingsValues,
} from "@/lib/register-settings";

export function useRegisterSettingsSave(initial: RegisterSettingsValues) {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [values, setValues] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [isSaving, setIsSaving] = useState(false);

  const isDirty = hasRegisterSettingsChanges(values, baseline);

  function issueMessage(issue: RegisterSettingsIssue): string {
    switch (issue) {
      case "passwordMinLengthRange":
        return t("passwordMinLengthRange");
    }
  }

  function conflictMessage(error: ApiError): string {
    switch (error.param) {
      case "password_min_length":
        return t("passwordMinLengthRange");
      default:
        return t("saveFailed");
    }
  }

  function setEnabled(checked: boolean) {
    setValues((previous) => ({ ...previous, enabled: checked }));
  }

  function setEmailDomainControlEnabled(checked: boolean) {
    setValues((previous) => ({
      ...previous,
      emailDomainControlEnabled: checked,
    }));
  }

  function updateAllowedEmailDomains(value: string) {
    setValues((previous) => ({ ...previous, allowedEmailDomains: value }));
  }

  function setPasswordRequirementsEnabled(checked: boolean) {
    setValues((previous) => ({
      ...previous,
      passwordRequirementsEnabled: checked,
    }));
  }

  function updatePasswordMinLength(value: string) {
    setValues((previous) => ({ ...previous, passwordMinLength: value }));
  }

  function updatePasswordComplexity(value: string) {
    setValues((previous) => ({ ...previous, passwordComplexity: value }));
  }

  async function save() {
    if (isSaving || !isDirty) {
      return;
    }

    const issue = validateRegisterSettings(values, baseline);

    if (issue !== null) {
      toast.error(issueMessage(issue));
      return;
    }

    const patch = buildRegisterSettingsPatch(values, baseline);

    setIsSaving(true);
    try {
      let next = { ...values };

      if (patch.register !== undefined) {
        const payload = await apiRequest("/api/v1/admin/settings/register", {
          method: "PATCH",
          body: patch.register,
        });
        const parsed = parseRegisterGroupValues(payload);

        if (parsed === null) {
          toast.error(t("saveFailed"));
          return;
        }

        next = { ...next, ...parsed };
      }

      if (patch.auth !== undefined) {
        const payload = await apiRequest("/api/v1/admin/settings/auth", {
          method: "PATCH",
          body: patch.auth,
        });
        const parsed = parseAuthGroupValues(payload);

        if (parsed === null) {
          toast.error(t("saveFailed"));
          return;
        }

        next = { ...next, ...parsed };
      }

      setValues(next);
      setBaseline(next);
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
    setEnabled,
    setEmailDomainControlEnabled,
    updateAllowedEmailDomains,
    setPasswordRequirementsEnabled,
    updatePasswordMinLength,
    updatePasswordComplexity,
    save,
  };
}
