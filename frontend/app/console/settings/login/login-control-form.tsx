"use client";

import { type FormEvent } from "react";
import { CalendarClock, Hourglass, KeyRound, ShieldUser } from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { useLoginSettingsSave } from "@/hooks/use-login-settings-save";
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";
import {
  SESSION_MAX_AGE_DAYS_MAX,
  SESSION_MAX_AGE_DAYS_MIN,
  type LoginSettingsValues,
} from "@/lib/login-settings";

export function LoginControlForm({
  settings,
}: {
  settings: LoginSettingsValues;
}) {
  const t = useTranslations("settings");
  const {
    values,
    isDirty,
    isSaving,
    setSessionMaxAgeEnabled,
    updateSessionMaxAgeDays,
    setTotpEnabled,
    setPasswordResetEnabled,
    save,
  } = useLoginSettingsSave(settings);
  const sessionMaxAgeExpansion = useSwitchRowExpansion(
    values.sessionMaxAgeEnabled,
  );

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  return (
    <form
      className="flex flex-col gap-4"
      method="post"
      onSubmit={handleSubmit}
      noValidate
    >
      <ConsoleSettingsGroup title={t("sessionGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("sessionMaxAgeEnabled")}
              checked={values.sessionMaxAgeEnabled}
              onCheckedChange={(checked) => {
                setSessionMaxAgeEnabled(checked);
                sessionMaxAgeExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={sessionMaxAgeExpansion.expanded}
          expandedContent={
            <ConsoleSettingsRow
              control={
                <div className="flex items-center gap-2">
                  <Input
                    aria-label={t("sessionMaxAge")}
                    className="w-28"
                    max={SESSION_MAX_AGE_DAYS_MAX}
                    min={SESSION_MAX_AGE_DAYS_MIN}
                    onChange={(event) =>
                      updateSessionMaxAgeDays(event.target.value)
                    }
                    type="number"
                    value={values.sessionMaxAgeDays}
                    disabled={isSaving}
                  />
                  <span className="text-sm text-muted-foreground">
                    {t("sessionMaxAgeUnitDay")}
                  </span>
                </div>
              }
              icon={<CalendarClock />}
              label={t("sessionMaxAge")}
            />
          }
          icon={<Hourglass />}
          label={t("sessionMaxAgeEnabled")}
          onExpandedChange={sessionMaxAgeExpansion.setExpanded}
        />
      </ConsoleSettingsGroup>
      <ConsoleSettingsGroup title={t("identityGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("twoFactorAuth")}
              checked={values.totpEnabled}
              onCheckedChange={setTotpEnabled}
              disabled={isSaving}
            />
          }
          icon={<ShieldUser />}
          label={t("twoFactorAuth")}
        />
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("passwordReset")}
              checked={values.passwordResetEnabled}
              onCheckedChange={setPasswordResetEnabled}
              disabled={isSaving}
            />
          }
          icon={<KeyRound />}
          label={t("passwordReset")}
        />
      </ConsoleSettingsGroup>
      <ConsoleFloatingSaveButton
        isDirty={isDirty}
        isPending={isSaving}
        label={t("save")}
      />
    </form>
  );
}
