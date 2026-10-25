import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  buildCaptchaSettingsPatch,
  hasCaptchaSettingsChanges,
  parseCaptchaSettingsValues,
  type CaptchaSettingsValues,
} from "@/lib/captcha-settings";

export function useCaptchaSettingsSave(initial: CaptchaSettingsValues) {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [values, setValues] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [isSaving, setIsSaving] = useState(false);

  const isDirty = hasCaptchaSettingsChanges(values, baseline);

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

    setIsSaving(true);
    try {
      const payload = await apiRequest("/api/v1/admin/settings/captcha", {
        method: "PATCH",
        body: buildCaptchaSettingsPatch(values, baseline),
      });
      const parsed = parseCaptchaSettingsValues(payload);

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
    setCaptchaEnabled,
    setCaptchaProvider,
    setTurnstileSiteKey,
    setTurnstileSecretKey,
    save,
  };
}
