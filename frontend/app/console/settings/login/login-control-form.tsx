"use client";

import { useState, type FormEvent } from "react";
import {
  CalendarClock,
  Fingerprint,
  Hourglass,
  KeyRound,
  ShieldCheck,
  ShieldUser,
} from "lucide-react";
import { useTranslations } from "next-intl";

import { ConsoleFloatingSaveButton } from "@/components/console/console-floating-save-button";
import { ConsoleSettingsGroup } from "@/components/console/console-settings-group";
import { ConsoleSettingsRow } from "@/components/console/console-settings-row";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { useLoginSettingsSave } from "@/hooks/use-login-settings-save";
import { useSwitchRowExpansion } from "@/hooks/use-switch-row-expansion";
import {
  CAPTCHA_PROVIDER_BUILTIN,
  SESSION_MAX_AGE_DAYS_MAX,
  SESSION_MAX_AGE_DAYS_MIN,
  type LoginSettingsValues,
} from "@/lib/login-settings";

const SESSION_UNIT_DAY = "day";

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
    setCaptchaEnabled,
    setCaptchaProvider,
    save,
  } = useLoginSettingsSave(settings);
  const [sessionMaxAgeUnit, setSessionMaxAgeUnit] = useState(SESSION_UNIT_DAY);
  const sessionMaxAgeExpansion = useSwitchRowExpansion(
    values.sessionMaxAgeEnabled,
  );
  const captchaExpansion = useSwitchRowExpansion(values.captchaEnabled);

  const sessionUnitItems = {
    [SESSION_UNIT_DAY]: t("sessionMaxAgeUnitDay"),
  };
  const captchaProviderItems = {
    [CAPTCHA_PROVIDER_BUILTIN]: t("captchaProviderBuiltin"),
    ...(values.captchaProvider === CAPTCHA_PROVIDER_BUILTIN
      ? {}
      : { [values.captchaProvider]: values.captchaProvider }),
  };

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void save();
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={handleSubmit} noValidate>
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
                  <Select
                    items={sessionUnitItems}
                    value={sessionMaxAgeUnit}
                    onValueChange={(value) => {
                      if (value !== null) {
                        setSessionMaxAgeUnit(value);
                      }
                    }}
                    disabled={isSaving}
                  >
                    <SelectTrigger
                      aria-label={t("sessionMaxAgeUnitDay")}
                      className="w-24"
                      size="sm"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="min-w-0">
                      <SelectItem value={SESSION_UNIT_DAY}>
                        {t("sessionMaxAgeUnitDay")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
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
      <ConsoleSettingsGroup title={t("captchaGroup")}>
        <ConsoleSettingsRow
          control={
            <Switch
              aria-label={t("captchaEnabled")}
              checked={values.captchaEnabled}
              onCheckedChange={(checked) => {
                setCaptchaEnabled(checked);
                captchaExpansion.expandOnEnable(checked);
              }}
              disabled={isSaving}
            />
          }
          expanded={captchaExpansion.expanded}
          expandedContent={
            <ConsoleSettingsRow
              control={
                <Select
                  items={captchaProviderItems}
                  value={values.captchaProvider}
                  onValueChange={(value) => {
                    if (value !== null) {
                      setCaptchaProvider(value);
                    }
                  }}
                  disabled={isSaving}
                >
                  <SelectTrigger
                    aria-label={t("captchaProvider")}
                    className="w-45"
                    size="sm"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={CAPTCHA_PROVIDER_BUILTIN}>
                      {t("captchaProviderBuiltin")}
                    </SelectItem>
                    {values.captchaProvider ===
                    CAPTCHA_PROVIDER_BUILTIN ? null : (
                      <SelectItem value={values.captchaProvider}>
                        {values.captchaProvider}
                      </SelectItem>
                    )}
                  </SelectContent>
                </Select>
              }
              icon={<ShieldCheck />}
              label={t("captchaProvider")}
            />
          }
          icon={<Fingerprint />}
          label={t("captchaEnabled")}
          onExpandedChange={captchaExpansion.setExpanded}
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
