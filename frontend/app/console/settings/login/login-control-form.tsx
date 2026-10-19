"use client";

import { useState, type FormEvent } from "react";
import {
  Clock,
  Clock3,
  Fingerprint,
  KeyRound,
  ShieldCheck,
  Smartphone,
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
              onCheckedChange={setSessionMaxAgeEnabled}
              disabled={isSaving}
            />
          }
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
              icon={<Clock3 />}
              label={t("sessionMaxAge")}
            />
          }
          icon={<Clock />}
          label={t("sessionMaxAgeEnabled")}
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
          icon={<Smartphone />}
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
              onCheckedChange={setCaptchaEnabled}
              disabled={isSaving}
            />
          }
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
