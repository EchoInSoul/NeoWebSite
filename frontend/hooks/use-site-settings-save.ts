import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { ApiError, apiRequest, looksLikeServiceFailure } from "@/lib/api";
import {
  buildSiteSettingsPatch,
  parseSiteSettingsValues,
  type SiteSettingsValues,
} from "@/lib/site-settings";

export function useSiteSettingsSave(initial: SiteSettingsValues) {
  const t = useTranslations("settings");
  const tCommon = useTranslations("common");
  const router = useRouter();
  const [values, setValues] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [isSaving, setIsSaving] = useState(false);

  const isDirty =
    Object.keys(buildSiteSettingsPatch(values, baseline)).length > 0;

  function updateText(
    field: "title" | "subtitle" | "description" | "keywords" | "url",
    value: string,
  ) {
    setValues((previous) => ({ ...previous, [field]: value }));
  }

  function setBlockSearchEngines(checked: boolean) {
    setValues((previous) => ({ ...previous, blockSearchEngines: checked }));
  }

  async function save() {
    if (isSaving || !isDirty) {
      return;
    }

    setIsSaving(true);
    try {
      const payload = await apiRequest("/api/v1/admin/settings/site", {
        method: "PATCH",
        body: buildSiteSettingsPatch(values, baseline),
      });
      const parsed = parseSiteSettingsValues(payload);

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
    updateText,
    setBlockSearchEngines,
    save,
  };
}
